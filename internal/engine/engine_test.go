package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hgta/flashdown/internal/config"
)

/* ------------------------------------------------------------------- fixtures */

// patternBytes is deterministic on purpose: a failure diff is readable, and the
// same payload can be compared byte-for-byte after a resume.
func patternBytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*31 + 7)
	}
	return b
}

func testConfig(dir string) config.Config {
	cfg := config.Default()
	cfg.DownloadRoot = dir
	cfg.Categories = []config.Category{{ID: "test", Name: "test", Dir: dir, BuiltIn: true}}
	cfg.MaxConcurrent = 2
	cfg.MaxJets = 1
	cfg.Preallocate = config.PreallocateNever
	cfg.Retry = config.Retry{MaxAttempts: 3, BackoffSeconds: 0}
	return cfg
}

// serverOpts is what a test wants the fake server to do.
type serverOpts struct {
	payload    []byte
	etag       string
	noRange    bool // answer everything with 200, i.e. never honour Range
	chunkSize  int
	chunkDelay time.Duration
	// ignoreRangePastZero answers 200 only when the requested start is past zero,
	// which is how a mid-flight downgrade (2.8) is provoked: the probe still looks
	// rangeful.
	ignoreRangePastZero bool
	// failData answers 503 to the first N data requests (the probe does not count).
	failData int
	// stallAfterProbe sends headers and then goes silent.
	stallAfterProbe bool
}

type testServer struct {
	srv  *httptest.Server
	opts serverOpts
	etag sync.Mutex
	tag  string
	fail atomic.Int32

	mu     sync.Mutex
	ranges []string
	count  int

	inflight atomic.Int32
	peak     atomic.Int32
}

func newTestServer(t *testing.T, opts serverOpts) *testServer {
	t.Helper()
	if opts.chunkSize <= 0 {
		opts.chunkSize = 64 << 10
	}

	ts := &testServer{opts: opts, tag: opts.etag}
	ts.fail.Store(int32(opts.failData))
	ts.srv = httptest.NewServer(http.HandlerFunc(ts.handle))
	t.Cleanup(ts.srv.Close)
	return ts
}

func (s *testServer) URL() string { return s.srv.URL + "/file.bin" }

func (s *testServer) SetETag(v string) {
	s.etag.Lock()
	s.tag = v
	s.etag.Unlock()
}

// Requests returns the Range header of every request, in order. The first is
// always the probe.
func (s *testServer) Requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.ranges...)
}

// MaxConcurrent is the largest number of requests the server had in flight at
// once: the only honest way for a test to prove jets really overlap.
func (s *testServer) MaxConcurrent() int { return int(s.peak.Load()) }

func (s *testServer) handle(w http.ResponseWriter, r *http.Request) {
	cur := s.inflight.Add(1)
	defer s.inflight.Add(-1)
	for {
		peak := s.peak.Load()
		if cur <= peak || s.peak.CompareAndSwap(peak, cur) {
			break
		}
	}

	rng := r.Header.Get("Range")

	s.mu.Lock()
	s.count++
	index := s.count
	s.ranges = append(s.ranges, rng)
	s.mu.Unlock()

	data := s.opts.payload
	total := len(data)

	start, end, ranged := 0, total-1, false
	if rng != "" {
		if st, en, ok := parseRange(rng, total); ok {
			start, end, ranged = st, en, true
		}
	}

	// The probe asks for a single byte; everything else is a data request.
	isProbe := ranged && end == start

	if !isProbe {
		if left := s.fail.Add(-1); left >= 0 {
			http.Error(w, "boom", http.StatusServiceUnavailable)
			return
		}
	}

	if s.opts.noRange || (s.opts.ignoreRangePastZero && ranged && start > 0) {
		ranged = false
		start, end = 0, total-1
	}

	if s.opts.stallAfterProbe && index > 1 {
		w.Header().Set("Content-Length", strconv.Itoa(end-start+1))
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
		return
	}

	s.etag.Lock()
	tag := s.tag
	s.etag.Unlock()
	if tag != "" {
		w.Header().Set("ETag", tag)
	}

	if ranged {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, total))
		w.Header().Set("Content-Length", strconv.Itoa(end-start+1))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.Header().Set("Content-Length", strconv.Itoa(total))
		w.WriteHeader(http.StatusOK)
	}

	flusher, _ := w.(http.Flusher)
	for off := start; off <= end; {
		n := s.opts.chunkSize
		if remain := end - off + 1; n > remain {
			n = remain
		}
		if _, err := w.Write(data[off : off+n]); err != nil {
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
		off += n
		if s.opts.chunkDelay > 0 {
			time.Sleep(s.opts.chunkDelay)
		}
	}
}

func parseRange(header string, total int) (int, int, bool) {
	const prefix = "bytes="
	if !strings.HasPrefix(header, prefix) {
		return 0, 0, false
	}
	left, right, ok := strings.Cut(strings.TrimPrefix(header, prefix), "-")
	if !ok {
		return 0, 0, false
	}
	start, err := strconv.Atoi(left)
	if err != nil || start < 0 || start >= total {
		return 0, 0, false
	}
	end := total - 1
	if right != "" {
		if v, err := strconv.Atoi(right); err == nil && v < end {
			end = v
		}
	}
	return start, end, true
}

/* -------------------------------------------------------------------- helpers */

func newManagerTask(t *testing.T, ts *testServer, dir string) (*Manager, *Task) {
	t.Helper()

	m := New(testConfig(dir))
	task, err := m.Add(Spec{URL: ts.URL(), Dir: dir, Category: "test"})
	if err != nil {
		t.Fatalf("add task: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		m.Shutdown(ctx)
	})
	return m, task
}

func waitState(t *testing.T, task *Task, want State) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if task.State() == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	v := task.View()
	t.Fatalf("state = %q (want %q), error = %q", v.State, want, v.Error)
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func assertFile(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s: %d bytes, want %d; contents differ", path, len(got), len(want))
	}
}

func assertGone(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("%s should have been removed (err = %v)", path, err)
	}
}

/* ---------------------------------------------------------------- block maths */

func TestBlockSize(t *testing.T) {
	cases := []struct {
		total   int64
		maxJets int
		want    int64
	}{
		{total: 4 << 20, maxJets: 5, want: 256 << 10},   // ~102KB after alignment -> floor
		{total: 100 << 20, maxJets: 5, want: 2_621_440}, // 100MB/40 = 2.5MB, already aligned
		{total: 10 << 30, maxJets: 5, want: 8 << 20},    // ceiling
		{total: 100 << 20, maxJets: 0, want: 8 << 20},   // maxJets is clamped to 1
	}
	for _, c := range cases {
		if got := BlockSize(c.total, c.maxJets); got != c.want {
			t.Errorf("BlockSize(%d, %d) = %d, want %d", c.total, c.maxJets, got, c.want)
		}
	}
}

func TestPlanBlocksCoversFileExactly(t *testing.T) {
	const total = 3 << 20
	blocks := PlanBlocks(total, 2)
	if len(blocks) == 0 {
		t.Fatal("no blocks planned")
	}
	if blocks[0].Start != 0 {
		t.Errorf("first block starts at %d", blocks[0].Start)
	}
	if last := blocks[len(blocks)-1]; last.End != total {
		t.Errorf("last block ends at %d, want %d", last.End, total)
	}
	for i := 1; i < len(blocks); i++ {
		if blocks[i].Start != blocks[i-1].End {
			t.Fatalf("blocks %d and %d are not contiguous", i-1, i)
		}
	}
	if TotalDone(blocks) != 0 {
		t.Errorf("fresh plan reports progress")
	}
	if AllDone(blocks) {
		t.Error("fresh plan reports complete")
	}
}

func TestProgressHelpers(t *testing.T) {
	blocks := []Block{{Start: 0, End: 100, Done: 100}, {Start: 100, End: 200, Done: 40}}
	if got := TotalDone(blocks); got != 140 {
		t.Errorf("TotalDone = %d, want 140", got)
	}
	if AllDone(blocks) {
		t.Error("AllDone should be false")
	}
	if got := NextIncomplete(blocks); got != 1 {
		t.Errorf("NextIncomplete = %d, want 1", got)
	}
	if got := blocks[1].Offset(); got != 140 {
		t.Errorf("Offset = %d, want 140", got)
	}
}

/* ---------------------------------------------------------------------- names */

func TestSanitizeName(t *testing.T) {
	cases := map[string]string{
		`a:b*c?.zip`:  `a_b_c_.zip`,
		"  spaced  ":  "spaced",
		"trailing...": "trailing",
		`..`:          "",
		``:            "",
	}
	for in, want := range cases {
		if got := sanitizeName(in); got != want {
			t.Errorf("sanitizeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUniqueNameSuffixesCollisions(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.zip"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := uniqueName(dir, "a.zip"); got != "a (2).zip" {
		t.Errorf("uniqueName = %q, want %q", got, "a (2).zip")
	}
	if got := uniqueName(dir, "b.zip"); got != "b.zip" {
		t.Errorf("uniqueName = %q, want %q", got, "b.zip")
	}
}

/* ---------------------------------------------------------------------- probe */

func TestProbeReadsRangeAndTotal(t *testing.T) {
	payload := patternBytes(300 << 10)
	ts := newTestServer(t, serverOpts{payload: payload, etag: `"abc"`})

	pr, err := Probe(context.Background(), NewClient(), ts.URL())
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if pr.Size != int64(len(payload)) {
		t.Errorf("size = %d, want %d", pr.Size, len(payload))
	}
	if !pr.Resumable {
		t.Error("expected the server to look resumable")
	}
	if pr.ETag != `"abc"` {
		t.Errorf("etag = %q", pr.ETag)
	}
	if pr.Filename != "file.bin" {
		t.Errorf("filename = %q, want file.bin", pr.Filename)
	}
}

func TestProbeDetectsMissingRangeSupport(t *testing.T) {
	ts := newTestServer(t, serverOpts{payload: patternBytes(64 << 10), noRange: true})

	pr, err := Probe(context.Background(), NewClient(), ts.URL())
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if pr.Resumable {
		t.Error("a 200 answer to a range request must not be treated as resumable")
	}
	if pr.Size != 64<<10 {
		t.Errorf("size = %d, want %d", pr.Size, 64<<10)
	}
}

/* ------------------------------------------------------------------ downloads */

func TestDownloadCompletes(t *testing.T) {
	payload := patternBytes(600 << 10)
	ts := newTestServer(t, serverOpts{payload: payload, etag: `"v1"`, chunkSize: 128 << 10})

	m, task := newManagerTask(t, ts, t.TempDir())
	if err := m.Start(task.ID); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitState(t, task, StateFinished)

	final := filepath.Join(task.Dir, task.Name())
	if task.Name() != "file.bin" {
		t.Errorf("name = %q, want file.bin", task.Name())
	}
	assertFile(t, final, payload)

	dataPath, journalPath := Paths(task.Dir, task.Name())
	assertGone(t, dataPath)
	assertGone(t, journalPath)

	v := task.View()
	if v.Size != int64(len(payload)) || v.Done != v.Size {
		t.Errorf("view = %d/%d, want %d/%d", v.Done, v.Size, len(payload), len(payload))
	}
	if v.Speed != 0 {
		t.Errorf("speed should settle at 0 when finished, got %d", v.Speed)
	}
}

// newParallelManager builds a manager with a chosen connection count, since the
// shared fixture deliberately pins MaxJets to 1.
func newParallelManager(t *testing.T, dir string, jets int) *Manager {
	t.Helper()
	cfg := testConfig(dir)
	cfg.MaxJets = jets
	m := New(cfg)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		m.Shutdown(ctx)
	})
	return m
}

func TestParallelJetsUseSeveralConnections(t *testing.T) {
	payload := patternBytes(4 << 20)
	ts := newTestServer(t, serverOpts{
		payload:    payload,
		etag:       `"v1"`,
		chunkSize:  64 << 10,
		chunkDelay: 3 * time.Millisecond,
	})

	dir := t.TempDir()
	m := newParallelManager(t, dir, 4)
	task, err := m.Add(Spec{URL: ts.URL(), Dir: dir, Category: "test"})
	if err != nil {
		t.Fatalf("add task: %v", err)
	}

	if err := m.Start(task.ID); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitState(t, task, StateFinished)
	assertFile(t, filepath.Join(dir, task.Name()), payload)

	if got := ts.MaxConcurrent(); got < 2 {
		t.Errorf("server saw at most %d concurrent requests, want >= 2", got)
	}
	if got := task.View().Jets; got != 0 {
		t.Errorf("jets = %d after finishing, want 0", got)
	}
}

func TestParallelPauseThenResumeKeepsBytes(t *testing.T) {
	payload := patternBytes(8 << 20)
	ts := newTestServer(t, serverOpts{
		payload:    payload,
		etag:       `"v1"`,
		chunkSize:  64 << 10,
		chunkDelay: 8 * time.Millisecond,
	})

	dir := t.TempDir()
	m := newParallelManager(t, dir, 4)
	task, err := m.Add(Spec{URL: ts.URL(), Dir: dir, Category: "test"})
	if err != nil {
		t.Fatalf("add task: %v", err)
	}

	if err := m.Start(task.ID); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitUntil(t, "some progress", func() bool { return task.Done() > 0 })
	if err := m.Pause(task.ID); err != nil {
		t.Fatalf("pause: %v", err)
	}
	waitState(t, task, StatePaused)

	partial := task.Done()
	if partial <= 0 || partial >= int64(len(payload)) {
		t.Fatalf("partial = %d, want something in (0, %d)", partial, len(payload))
	}

	if err := m.Start(task.ID); err != nil {
		t.Fatalf("resume: %v", err)
	}
	waitState(t, task, StateFinished)
	assertFile(t, filepath.Join(dir, task.Name()), payload)
}

// 3.4: raising the count mid-flight must spawn more connections without a
// restart, and the file must still come out byte-perfect.
func TestMaxJetsTakesEffectMidFlight(t *testing.T) {
	payload := patternBytes(4 << 20)
	ts := newTestServer(t, serverOpts{
		payload:    payload,
		etag:       `"v1"`,
		chunkSize:  64 << 10,
		chunkDelay: 10 * time.Millisecond,
	})

	dir := t.TempDir()
	m := newParallelManager(t, dir, 1)
	task, err := m.Add(Spec{URL: ts.URL(), Dir: dir, Category: "test"})
	if err != nil {
		t.Fatalf("add task: %v", err)
	}

	if err := m.Start(task.ID); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitUntil(t, "some progress", func() bool { return task.Done() > 0 })

	if got := m.SetMaxJets(4); got != 4 {
		t.Fatalf("SetMaxJets(4) = %d", got)
	}
	waitState(t, task, StateFinished)
	assertFile(t, filepath.Join(dir, task.Name()), payload)

	if got := ts.MaxConcurrent(); got < 2 {
		t.Errorf("server saw at most %d concurrent requests after raising the count", got)
	}
}

// Release hosting answers with a 302 to a signed URL on another host, and that
// host is sometimes the only one reachable. Every block must go straight there
// instead of paying for the redirect again.
func TestRedirectTargetIsUsedForBlocks(t *testing.T) {
	payload := patternBytes(600 << 10)
	cdn := newTestServer(t, serverOpts{payload: payload, etag: `"v1"`, chunkSize: 128 << 10})

	var originHits atomic.Int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		originHits.Add(1)
		http.Redirect(w, r, cdn.URL(), http.StatusFound)
	}))
	t.Cleanup(origin.Close)

	dir := t.TempDir()
	m := New(testConfig(dir))
	task, err := m.Add(Spec{URL: origin.URL + "/file.bin", Dir: dir, Category: "test"})
	if err != nil {
		t.Fatalf("add task: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		m.Shutdown(ctx)
	})

	if err := m.Start(task.ID); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitState(t, task, StateFinished)
	assertFile(t, filepath.Join(dir, task.Name()), payload)

	if got := originHits.Load(); got != 1 {
		t.Errorf("origin was asked %d times, want 1 (the probe)", got)
	}
}

// A signed URL that outlives its window answers 403. The plan is still valid, so
// the engine should ask the origin for a fresh address instead of failing.
func TestExpiredSignedURLIsRefreshed(t *testing.T) {
	payload := patternBytes(600 << 10)
	inner := newTestServer(t, serverOpts{payload: payload, etag: `"v1"`, chunkSize: 128 << 10})

	var expired atomic.Bool
	expired.Store(true)

	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isProbe := r.Header.Get("Range") == "bytes=0-0"
		if !isProbe && expired.Swap(false) {
			http.Error(w, "signature expired", http.StatusForbidden)
			return
		}
		inner.handle(w, r)
	}))
	t.Cleanup(cdn.Close)

	var originHits atomic.Int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		originHits.Add(1)
		http.Redirect(w, r, cdn.URL+"/file.bin", http.StatusFound)
	}))
	t.Cleanup(origin.Close)

	dir := t.TempDir()
	m := New(testConfig(dir))
	task, err := m.Add(Spec{URL: origin.URL + "/file.bin", Dir: dir, Category: "test"})
	if err != nil {
		t.Fatalf("add task: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		m.Shutdown(ctx)
	})

	if err := m.Start(task.ID); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitState(t, task, StateFinished)
	assertFile(t, filepath.Join(dir, task.Name()), payload)

	if got := originHits.Load(); got != 2 {
		t.Errorf("origin was asked %d times, want 2 (probe + refresh)", got)
	}
	var refreshed bool
	for _, line := range task.Detail().Log {
		if strings.Contains(line, "重新获取") {
			refreshed = true
		}
	}
	if !refreshed {
		t.Errorf("log should mention the refresh: %v", task.Detail().Log)
	}
}

func TestPauseThenResumeContinuesFromOffset(t *testing.T) {
	payload := patternBytes(2 << 20)
	ts := newTestServer(t, serverOpts{
		payload:    payload,
		etag:       `"v1"`,
		chunkSize:  64 << 10,
		chunkDelay: 20 * time.Millisecond,
	})

	m, task := newManagerTask(t, ts, t.TempDir())
	if err := m.Start(task.ID); err != nil {
		t.Fatalf("start: %v", err)
	}

	waitUntil(t, "some progress", func() bool { return task.Done() > 0 })
	if err := m.Pause(task.ID); err != nil {
		t.Fatalf("pause: %v", err)
	}
	waitState(t, task, StatePaused)

	partial := task.Done()
	if partial <= 0 {
		t.Fatal("nothing was written before the pause")
	}
	if partial >= int64(len(payload)) {
		t.Fatal("the download finished before the pause took effect; test is not meaningful")
	}

	// The journal is what a later process would resume from.
	_, journalPath := Paths(task.Dir, task.Name())
	if _, err := os.Stat(journalPath); err != nil {
		t.Fatalf("journal missing after pause: %v", err)
	}

	before := len(ts.Requests())
	if err := m.Start(task.ID); err != nil {
		t.Fatalf("restart: %v", err)
	}
	waitState(t, task, StateFinished)

	requests := ts.Requests()[before:]
	if len(requests) < 2 {
		t.Fatalf("expected a probe and a data request, got %v", requests)
	}
	start, _, ok := parseRange(requests[1], len(payload))
	if !ok {
		t.Fatalf("second request is not a range request: %q", requests[1])
	}
	if start == 0 {
		t.Errorf("resume started at 0 instead of %d: %v", partial, requests[1])
	}

	assertFile(t, filepath.Join(task.Dir, "file.bin"), payload)
}

func TestRangeDowngradeRestartsFromZero(t *testing.T) {
	payload := patternBytes(1 << 20)
	ts := newTestServer(t, serverOpts{
		payload:             payload,
		etag:                `"v1"`,
		chunkSize:           64 << 10,
		chunkDelay:          10 * time.Millisecond,
		ignoreRangePastZero: true,
	})

	m, task := newManagerTask(t, ts, t.TempDir())
	if err := m.Start(task.ID); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitUntil(t, "progress", func() bool { return task.Done() > 0 })
	_ = m.Pause(task.ID)
	waitState(t, task, StatePaused)

	// Resuming now makes the server ignore the range, which must not corrupt the
	// file: the engine has to fall back to one connection from byte zero.
	if err := m.Start(task.ID); err != nil {
		t.Fatalf("restart: %v", err)
	}
	waitState(t, task, StateFinished)

	assertFile(t, filepath.Join(task.Dir, "file.bin"), payload)
	if v := task.View(); v.Resumable {
		t.Error("resumable should be cleared after a downgrade")
	}
}

func TestTransientFailureIsRetried(t *testing.T) {
	payload := patternBytes(256 << 10)
	ts := newTestServer(t, serverOpts{payload: payload, etag: `"v1"`, failData: 1})

	m, task := newManagerTask(t, ts, t.TempDir())
	if err := m.Start(task.ID); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitState(t, task, StateFinished)

	assertFile(t, filepath.Join(task.Dir, "file.bin"), payload)
	if got := len(ts.Requests()); got < 3 {
		t.Errorf("expected probe + 503 + successful retry, got %d requests", got)
	}
}

func TestStalledDownloadIsRetriedThenFails(t *testing.T) {
	oldWindow, oldCheck := stallWindow, stallCheck
	stallWindow, stallCheck = 250*time.Millisecond, 40*time.Millisecond
	defer func() { stallWindow, stallCheck = oldWindow, oldCheck }()

	payload := patternBytes(128 << 10)
	ts := newTestServer(t, serverOpts{payload: payload, etag: `"v1"`, stallAfterProbe: true})

	m, task := newManagerTask(t, ts, t.TempDir())
	if err := m.Start(task.ID); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitState(t, task, StateError)

	v := task.View()
	if !strings.Contains(v.Error, "重试") {
		t.Errorf("error = %q, want a message about exhausting retries", v.Error)
	}
}

func TestRemoteChangeIsRefused(t *testing.T) {
	payload := patternBytes(1 << 20)
	ts := newTestServer(t, serverOpts{
		payload:    payload,
		etag:       `"v1"`,
		chunkSize:  64 << 10,
		chunkDelay: 15 * time.Millisecond,
	})

	m, task := newManagerTask(t, ts, t.TempDir())
	if err := m.Start(task.ID); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitUntil(t, "progress", func() bool { return task.Done() > 0 })
	_ = m.Pause(task.ID)
	waitState(t, task, StatePaused)

	ts.SetETag(`"v2"`)
	if err := m.Start(task.ID); err != nil {
		t.Fatalf("restart: %v", err)
	}
	waitState(t, task, StateError)

	if v := task.View(); !strings.Contains(v.Error, "ETag") {
		t.Errorf("error = %q, want it to name the ETag mismatch", v.Error)
	}

	// The escape hatch: discarding the progress downloads a fresh copy.
	if err := m.Restart(task.ID); err != nil {
		t.Fatalf("restart from scratch: %v", err)
	}
	waitState(t, task, StateFinished)
	assertFile(t, filepath.Join(task.Dir, "file.bin"), payload)
}

func TestRestoreRebuildsPausedTask(t *testing.T) {
	payload := patternBytes(1 << 20)
	ts := newTestServer(t, serverOpts{
		payload:    payload,
		etag:       `"v1"`,
		chunkSize:  64 << 10,
		chunkDelay: 15 * time.Millisecond,
	})

	dir := t.TempDir()
	cfg := testConfig(dir)

	m := New(cfg)
	task, err := m.Add(Spec{URL: ts.URL(), Dir: dir, Category: "test"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := m.Start(task.ID); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitUntil(t, "progress", func() bool { return task.Done() > 0 })
	_ = m.Pause(task.ID)
	waitState(t, task, StatePaused)

	partial := task.Done()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m.Shutdown(ctx)

	// A fresh manager, as if the process had been restarted.
	m2 := New(cfg)
	if err := m2.Restore(); err != nil {
		t.Fatalf("restore: %v", err)
	}

	tasks := m2.Tasks()
	if len(tasks) != 1 {
		t.Fatalf("restored %d tasks, want 1", len(tasks))
	}
	restored := tasks[0]
	v := restored.View()
	if v.State != StatePaused {
		t.Errorf("restored state = %q, want paused", v.State)
	}
	if v.Done != partial {
		t.Errorf("restored progress = %d, want %d", v.Done, partial)
	}
	if v.Name != "file.bin" || v.Size != int64(len(payload)) {
		t.Errorf("restored metadata = %q/%d", v.Name, v.Size)
	}

	// And it must still finish correctly.
	if err := m2.Start(restored.ID); err != nil {
		t.Fatalf("start restored: %v", err)
	}
	waitState(t, restored, StateFinished)
	assertFile(t, filepath.Join(dir, "file.bin"), payload)
}

// The CLI downloads into whatever -o names, which need not be one of the
// configured directories, so recovery has to be possible for one directory at a
// time. Without it a second run of the same command starts from byte zero.
func TestRestoreDirFindsTaskOutsideConfig(t *testing.T) {
	payload := patternBytes(1 << 20)
	ts := newTestServer(t, serverOpts{
		payload:    payload,
		etag:       `"v1"`,
		chunkSize:  64 << 10,
		chunkDelay: 15 * time.Millisecond,
	})

	dir := t.TempDir()
	m := New(testConfig(dir))
	task, err := m.Add(Spec{URL: ts.URL(), Dir: dir, Category: "test"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := m.Start(task.ID); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitUntil(t, "progress", func() bool { return task.Done() > 0 })
	_ = m.Pause(task.ID)
	waitState(t, task, StatePaused)
	partial := task.Done()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m.Shutdown(ctx)

	// A fresh manager whose configured download root is somewhere else entirely.
	m2 := New(testConfig(t.TempDir()))
	if err := m2.RestoreDir(dir); err != nil {
		t.Fatalf("restore dir: %v", err)
	}
	restored := m2.Find(ts.URL(), dir)
	if restored == nil {
		t.Fatal("Find did not return the restored task")
	}
	if got := restored.Done(); got != partial || got <= 0 {
		t.Fatalf("restored progress = %d, want %d", got, partial)
	}

	if err := m2.Start(restored.ID); err != nil {
		t.Fatalf("start restored: %v", err)
	}
	waitState(t, restored, StateFinished)
	assertFile(t, filepath.Join(dir, restored.Name()), payload)
}

/* -------------------------------------------------------------------- journal */

func TestJournalRoundTrip(t *testing.T) {
	dir := t.TempDir()
	_, journalPath := Paths(dir, "x.bin")

	j := &Journal{
		URL:       "https://example.com/x.bin",
		Name:      "x.bin",
		Size:      1000,
		ETag:      `"e"`,
		Modified:  "Mon, 01 Jan 2026 00:00:00 GMT",
		Resumable: true,
		Blocks:    []Block{{Start: 0, End: 500, Done: 200}, {Start: 500, End: 1000}},
	}
	if err := j.Save(journalPath); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := LoadJournal(journalPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Done() != 200 {
		t.Errorf("Done = %d, want 200", got.Done())
	}
	if got.ETag != `"e"` || !got.Resumable || len(got.Blocks) != 2 {
		t.Errorf("round trip lost fields: %+v", got)
	}
}

func TestCorruptJournalIsBackedUp(t *testing.T) {
	dir := t.TempDir()
	_, journalPath := Paths(dir, "x.bin")
	if err := os.WriteFile(journalPath, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadJournal(journalPath)
	if !errors.Is(err, ErrJournalCorrupt) {
		t.Fatalf("err = %v, want ErrJournalCorrupt", err)
	}
	if _, statErr := os.Stat(journalPath + ".bak"); statErr != nil {
		t.Errorf("damaged journal was not kept: %v", statErr)
	}
}

/* --------------------------------------------------------------------- errors */

func TestClassify(t *testing.T) {
	cases := []struct {
		err  error
		want ErrorKind
	}{
		{&StatusError{Code: 503, Status: "503 Service Unavailable"}, Retryable},
		{&StatusError{Code: 429, Status: "429 Too Many Requests"}, Retryable},
		{&StatusError{Code: 404, Status: "404 Not Found"}, Terminal},
		{&StatusError{Code: 403, Status: "403 Forbidden"}, Terminal},
		{ErrStalled, Retryable},
		{errors.New("connection reset by peer"), Retryable},
		{errors.New("nope"), Terminal},
		{context.Canceled, Terminal},
	}
	for _, c := range cases {
		if got := Classify(c.err); got != c.want {
			t.Errorf("Classify(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}
