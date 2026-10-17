package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

const (
	// readBuffer is 256 KiB: big enough that syscall overhead disappears, small
	// enough that a pause takes effect immediately (2.4).
	readBuffer = 256 << 10
	// journalInterval is D9's "every 1-3 seconds".
	journalInterval = time.Second
	// renameAttempts covers the antivirus-holds-the-handle case from the risk
	// table: a few quick retries usually outlast a scanner.
	renameAttempts = 5
)

// stallWindow is 2.10: sixty seconds of silence is a failure, not patience.
//
// Both are variables rather than constants so the test suite can shrink the
// window instead of waiting a real minute for a stall.
var (
	stallWindow = 60 * time.Second
	stallCheck  = 5 * time.Second
)

// errRangeDowngrade means the server answered a ranged request with 200, so it
// is sending the whole file from byte zero (2.8).
var errRangeDowngrade = errors.New("server ignored the range request")

// runState is the bookkeeping shared by the stream loop and the retry loop: the
// writer, where to flush, and the sampling window used for the speed readout.
type runState struct {
	w         *Writer
	journal   string
	lastFlush time.Time
	sampleAt  time.Time
	sampleAtB int64
	mark      time.Time
}

// flush publishes speed and, at most once a second, persists progress.
//
// The order matters: Sync the data first, then write the journal. A journal that
// claims bytes which are still in the page cache would resume into a hole.
func (rs *runState) flush(t *Task, force bool) {
	now := time.Now()

	if elapsed := now.Sub(rs.sampleAt); elapsed >= time.Second {
		done := t.Done()
		delta := done - rs.sampleAtB
		rs.sampleAt = now
		rs.sampleAtB = done
		t.setSpeed(int64(float64(delta) / elapsed.Seconds()))
	}

	if !force && now.Sub(rs.lastFlush) < journalInterval {
		return
	}
	// An empty path means the plan is not resumable anyway (unknown size), so
	// there is nothing worth recording.
	if rs.journal != "" {
		if err := rs.w.Sync(); err == nil {
			_ = t.journal().Save(rs.journal)
		}
	}
	rs.lastFlush = now

	t.addElapsed(now.Sub(rs.mark))
	rs.mark = now
}

// download is one full run: probe, plan, stream, finish.
func (m *Manager) download(ctx context.Context, t *Task) {
	cfg := m.Config()

	// The slot is what turns "start everything" into a queue instead of a fork
	// bomb (3.7). Waiting here is exactly what the waiting state means.
	t.setState(StateWaiting)
	if err := m.acquireSlot(ctx); err != nil {
		return
	}
	defer m.releaseSlot()

	mark := time.Now()
	defer func() { t.addElapsed(time.Since(mark)) }()

	// ---- 1. ask the server (2.2)
	t.setState(StateProbing)
	pr, err := Probe(ctx, m.client, t.URL)
	if err != nil {
		m.halted(ctx, t, err)
		return
	}
	if t.Name() == "" {
		t.setName(sanitizeOr(pr.Filename, "download"))
	}
	dataPath, journalPath := Paths(t.Dir, t.Name())

	resume := "不支持续传"
	if pr.Resumable {
		resume = "支持断点续传"
	}
	sizeText := "大小未知"
	if pr.Size > 0 {
		sizeText = fmt.Sprintf("%d 字节", pr.Size)
	}
	t.logf("探测完成：%s，%s", sizeText, resume)

	// ---- 2. reconcile the plan with whatever is already on disk (2.7 / 2.9)
	blocks, restart, reason, err := reconcile(t, pr, cfg.MaxJets)
	if err != nil {
		t.setError(err)
		return
	}
	if reason != "" {
		t.logf("%s，从头开始下载", reason)
	}

	// ---- 3. a size the server will not state cannot be resumed or planned
	if pr.Size <= 0 {
		m.downloadUnknownSize(ctx, t, dataPath, journalPath)
		return
	}

	w, err := OpenWriter(dataPath, pr.Size, cfg.Preallocate)
	if err != nil {
		t.setError(err)
		return
	}
	defer w.Close()
	if w.Preallocated() {
		t.logf("已预分配 %d 字节", pr.Size)
	}

	t.prepare(pr.Size, pr.ETag, pr.Modified, pr.Resumable && !restart, blocks)

	rs := &runState{
		w:         w,
		journal:   journalPath,
		lastFlush: time.Now(),
		sampleAt:  time.Now(),
		sampleAtB: t.Done(),
		mark:      time.Now(),
	}
	// Persist the initial plan immediately: if the process dies during the first
	// second, the next run must still know the block layout.
	rs.flush(t, true)

	// ---- 4. stream, retrying transient failures (2.11)
	attempts := 0
	maxAttempts := cfg.Retry.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	for {
		if ctx.Err() != nil {
			m.pause(t, rs, journalPath)
			return
		}

		idx := NextIncomplete(blocks)
		if idx < 0 {
			break
		}

		t.setState(StateDownloading)
		t.setJets([]Jet{{Index: 0, Start: t.blockAt(idx).Offset(), End: t.blockAt(idx).End}})

		err := m.stream(ctx, t, rs, idx)

		switch {
		case err == nil:
			attempts = 0
			continue

		case errors.Is(err, errRangeDowngrade):
			// 2.8: fall back to one connection from the beginning, and say so -
			// the user needs to know this file can no longer survive an interruption.
			t.logf("服务器忽略了 Range 请求，降级为单连接下载")
			blocks = SingleBlock(pr.Size)
			if terr := w.Truncate(0); terr != nil {
				t.setError(terr)
				return
			}
			t.prepare(pr.Size, pr.ETag, pr.Modified, false, blocks)
			rs.sampleAtB = 0
			attempts = 0
			continue

		case ctx.Err() != nil:
			m.pause(t, rs, journalPath)
			return
		}

		// A failure that might pass on its own: back off, then try again.
		if Classify(err) == Terminal {
			t.setError(err)
			return
		}
		attempts++
		if attempts > maxAttempts {
			t.setError(fmt.Errorf("重试 %d 次后仍然失败：%w", maxAttempts, err))
			return
		}

		backoff := time.Duration(cfg.Retry.BackoffSeconds) * time.Second * time.Duration(attempts)
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
		t.logf("%v（%d/%d），%s 后重试", err, attempts, maxAttempts, backoff)
		t.setSpeed(0)

		select {
		case <-ctx.Done():
			m.pause(t, rs, journalPath)
			return
		case <-time.After(backoff):
		}
	}

	m.finish(t, rs, dataPath, journalPath)
}

// reconcile decides what to do with existing progress.
//
// Three outcomes: resume the journal's plan, discard it and start over (a
// capability downgrade), or refuse because the remote is no longer the same file
// (2.9). Refusing is deliberate - silently splicing two different files together
// produces a corrupt download that looks successful.
func reconcile(t *Task, pr ProbeResult, maxJets int) (blocks []Block, restart bool, reason string, err error) {
	cur := t.journal()

	if TotalDone(cur.Blocks) <= 0 {
		// Nothing on disk worth keeping.
		if pr.Resumable {
			return PlanBlocks(pr.Size, maxJets), false, "", nil
		}
		return SingleBlock(pr.Size), false, "", nil
	}

	// Fingerprints first: an ETag mismatch means these bytes belong to another
	// version of the file.
	if cur.ETag != "" && pr.ETag != "" && cur.ETag != pr.ETag {
		return nil, false, "", fmt.Errorf("%w（ETag %s → %s）", ErrRemoteChanged, short(cur.ETag), short(pr.ETag))
	}
	if cur.Modified != "" && pr.Modified != "" && cur.Modified != pr.Modified {
		return nil, false, "", fmt.Errorf("%w（Last-Modified %s → %s）", ErrRemoteChanged, cur.Modified, pr.Modified)
	}
	if cur.Size > 0 && pr.Size > 0 && cur.Size != pr.Size {
		return nil, false, "", fmt.Errorf("%w（大小 %d → %d）", ErrRemoteChanged, cur.Size, pr.Size)
	}
	if cur.ETag == "" && cur.Modified == "" {
		t.logf("警告：服务器未提供 ETag / Last-Modified，续传无法校验文件是否变更")
	}

	// The bookkeeping is intact but the server will not do ranges any more: the
	// only way forward is a single connection from byte zero.
	if !pr.Resumable {
		return SingleBlock(pr.Size), true, "服务器不再支持 Range", nil
	}

	return cur.Blocks, false, "", nil
}

// stream fetches block idx, writing positionally as bytes arrive.
func (m *Manager) stream(ctx context.Context, t *Task, rs *runState, idx int) error {
	block := t.blockAt(idx)
	offset := block.Offset()
	if offset >= block.End {
		return nil
	}

	// Claim the block for the duration of the request so the block map can show
	// it as in flight; M1 has exactly one connection, hence jet 0.
	t.setBlockJet(idx, 0)
	defer t.setBlockJet(idx, -1)

	reqCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, t.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, block.End-1))
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("User-Agent", UserAgent)

	guard := newStallGuard(cancel)
	defer guard.stop()

	resp, err := m.client.Do(req)
	if err != nil {
		if guard.Stalled() {
			return ErrStalled
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	defer func() {
		// Drain a little so the connection can be reused, then close.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 32<<10))
		_ = resp.Body.Close()
	}()

	switch resp.StatusCode {
	case http.StatusPartialContent:
		// Check that the server started where we asked: a wrong start would write
		// the wrong bytes at the wrong offset and still look like progress.
		if start, ok := contentRangeStart(resp.Header.Get("Content-Range")); !ok || start != offset {
			return fmt.Errorf("服务器返回了错误的字节区间（期望 %d）：%q", offset,
				resp.Header.Get("Content-Range"))
		}

	case http.StatusRequestedRangeNotSatisfiable:
		// Everything in this block is already present; treat it as done.
		t.forceComplete(idx)
		return nil

	case http.StatusOK:
		// A 200 to a range request means the server is sending the whole file. At
		// offset zero that is exactly what this block wants, so consume it; past
		// zero the response would splatter bytes over the wrong offsets, so the
		// plan has to be rebuilt (2.8). This is also what stops a downgrade from
		// looping: after it, every block starts at zero.
		if offset > 0 {
			return errRangeDowngrade
		}

	default:
		return statusOf(resp)
	}

	buf := make([]byte, readBuffer)
	for offset < block.End {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			// Never write past the block even if the server over-delivers.
			if int64(n) > block.End-offset {
				n = int(block.End - offset)
			}
			if _, werr := rs.w.WriteAt(buf[:n], offset); werr != nil {
				return werr
			}
			offset += int64(n)
			t.advance(idx, int64(n))
			guard.touch()
			rs.flush(t, false)
		}

		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				break
			}
			if guard.Stalled() {
				return ErrStalled
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return rerr
		}
	}

	if offset < block.End {
		// The body ended early without an error: a partial read, not a pause.
		return io.ErrUnexpectedEOF
	}

	rs.flush(t, true)
	t.logf("分块 %d/%d 完成（%d 字节）", idx+1, t.BlockCount(), block.Len())
	return nil
}

// downloadUnknownSize handles a response with no Content-Length (chunked).
//
// Such a file cannot be planned into blocks and cannot be resumed - there is no
// size to resume against and no way to know the final length until EOF - so it is
// written sequentially and the size is only filled in at the end.
func (m *Manager) downloadUnknownSize(ctx context.Context, t *Task, dataPath, journalPath string) {
	// The size is unknown up front, so preallocation cannot apply, and journaling
	// would only record a plan that no later run could resume against.
	_ = journalPath

	w, err := OpenWriter(dataPath, 0, PreallocateNever)
	if err != nil {
		t.setError(err)
		return
	}
	defer w.Close()

	t.prepare(0, "", "", false, nil)
	t.setState(StateDownloading)

	reqCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, t.URL, nil)
	if err != nil {
		t.setError(err)
		return
	}
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("User-Agent", UserAgent)

	guard := newStallGuard(cancel)
	defer guard.stop()

	resp, err := m.client.Do(req)
	if err != nil {
		m.halted(ctx, t, err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.setError(statusOf(resp))
		return
	}

	// No journal: without a size there is nothing a later run could resume
	// against, so recording one would only create a file that restore must skip.
	rs := &runState{w: w, journal: "", lastFlush: time.Now(), sampleAt: time.Now(), mark: time.Now()}

	var offset int64
	buf := make([]byte, readBuffer)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.WriteAt(buf[:n], offset); werr != nil {
				t.setError(werr)
				return
			}
			offset += int64(n)
			t.setSize(offset)
			t.advance(-1, int64(n))
			guard.touch()
			rs.flush(t, false)
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				break
			}
			if guard.Stalled() {
				t.setError(ErrStalled)
				return
			}
			if ctx.Err() != nil {
				m.pause(t, rs, journalPath)
				return
			}
			t.setError(rerr)
			return
		}
	}

	// The size was only knowable now.
	blocks := SingleBlock(offset)
	t.prepare(offset, "", "", false, blocks)
	t.forceComplete(0)
	m.finish(t, rs, dataPath, journalPath)
}

// finish verifies, flushes, renames into place and cleans up (2.12).
func (m *Manager) finish(t *Task, rs *runState, dataPath, journalPath string) {
	t.setState(StateVerifying)

	size := t.Size()
	if size > 0 {
		if err := rs.w.Truncate(size); err != nil {
			t.setError(err)
			return
		}
	}
	if err := rs.w.Sync(); err != nil {
		t.setError(err)
		return
	}
	if err := rs.w.Close(); err != nil {
		t.setError(err)
		return
	}

	t.setState(StateCompleting)
	t.setJets(nil)
	t.setSpeed(0)

	final := uniqueName(t.Dir, t.Name())
	if err := renameWithRetry(dataPath, filepath.Join(t.Dir, final)); err != nil {
		t.setError(err)
		return
	}
	_ = RemoveJournal(journalPath)

	t.setName(final)
	t.setState(StateFinished)
	t.logf("下载完成：%s", final)
}

// pause persists what is on disk and leaves the task resumable (2.7).
func (m *Manager) pause(t *Task, rs *runState, journalPath string) {
	if rs != nil && rs.w != nil {
		// Data, then journal - never the other way round.
		if err := rs.w.Sync(); err == nil {
			_ = t.journal().Save(journalPath)
		}
		_ = rs.w.Close()
	}
	t.setJets(nil)
	t.setSpeed(0)
	t.setState(StatePaused)
	t.logf("已暂停，%d 字节已落盘", t.Done())
}

// halted turns a probe failure into either a pause or an error.
func (m *Manager) halted(ctx context.Context, t *Task, err error) {
	t.setJets(nil)
	t.setSpeed(0)
	if ctx.Err() != nil {
		t.setState(StatePaused)
		return
	}
	t.setError(err)
}

// forceComplete marks a block as fully present without transferring bytes.
func (t *Task) forceComplete(i int) {
	t.mu.Lock()
	if i >= 0 && i < len(t.blocks) {
		delta := t.blocks[i].Len() - t.blocks[i].Done
		t.blocks[i].Done = t.blocks[i].Len()
		t.done += delta
	}
	t.mu.Unlock()
}

// Done is the total bytes on disk across all blocks.
func (t *Task) Done() int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.done
}

// Size is the total file size, 0 while unknown.
func (t *Task) Size() int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.size
}

// BlockCount is the number of planned blocks.
func (t *Task) BlockCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.blocks)
}

func (t *Task) setSize(size int64) {
	t.mu.Lock()
	t.size = size
	t.mu.Unlock()
}

/* -------------------------------------------------------------------- helpers */

// stallGuard cancels a request that stops making progress (2.10).
type stallGuard struct {
	cancel  context.CancelFunc
	last    atomic.Int64 // unix nanos of the last byte received
	stalled atomic.Bool
	stopCh  chan struct{}
}

func newStallGuard(cancel context.CancelFunc) *stallGuard {
	g := &stallGuard{cancel: cancel, stopCh: make(chan struct{})}
	g.last.Store(time.Now().UnixNano())

	go func() {
		tick := time.NewTicker(stallCheck)
		defer tick.Stop()
		for {
			select {
			case <-g.stopCh:
				return
			case <-tick.C:
				if time.Since(time.Unix(0, g.last.Load())) > stallWindow {
					// Flag before cancelling: otherwise the resulting
					// context.Canceled is indistinguishable from a user pause.
					g.stalled.Store(true)
					g.cancel()
					return
				}
			}
		}
	}()
	return g
}

func (g *stallGuard) touch()        { g.last.Store(time.Now().UnixNano()) }
func (g *stallGuard) Stalled() bool { return g.stalled.Load() }
func (g *stallGuard) stop()         { close(g.stopCh) }

// contentRangeStart parses the start offset out of `bytes 100-199/1000`.
func contentRangeStart(header string) (int64, bool) {
	_, spec, ok := strings.Cut(header, " ")
	if !ok {
		spec = header
	}
	first, _, ok := strings.Cut(spec, "/")
	if !ok {
		return 0, false
	}
	startText, _, ok := strings.Cut(first, "-")
	if !ok {
		return 0, false
	}
	var start int64
	if _, err := fmt.Sscanf(strings.TrimSpace(startText), "%d", &start); err != nil {
		return 0, false
	}
	return start, true
}

// renameWithRetry outlasts a scanner that is holding the target open.
func renameWithRetry(from, to string) error {
	var err error
	for i := 1; i <= renameAttempts; i++ {
		if err = os.Rename(from, to); err == nil {
			return nil
		}
		time.Sleep(time.Duration(i) * 200 * time.Millisecond)
	}
	return fmt.Errorf("rename %s → %s: %w", filepath.Base(from), filepath.Base(to), err)
}

func sanitizeOr(name, fallback string) string {
	if name == "" {
		return fallback
	}
	return name
}

// short keeps a fingerprint readable in a one-line error.
func short(s string) string {
	s = strings.Trim(s, `"`)
	if len(s) > 16 {
		return s[:16] + "…"
	}
	return s
}
