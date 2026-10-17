package engine

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// State mirrors app's task states.
//
// The engine deliberately does not import app: that is what keeps it runnable
// without a window. The two sets of constants must agree on the wire strings.
type State string

const (
	StateDraft       State = "draft"
	StateWaiting     State = "waiting"
	StateProbing     State = "probing"
	StateDownloading State = "downloading"
	StatePaused      State = "paused"
	StateVerifying   State = "verifying"
	StateCompleting  State = "completing"
	StateFinished    State = "finished"
	StateError       State = "error"
	StateRemoved     State = "removed"
)

// logLimit keeps a long-running task from growing its log without bound.
const logLimit = 200

// Spec is what a caller must supply to create a task.
type Spec struct {
	URL      string
	Name     string // optional; probed from the server when empty
	Category string
	Dir      string // target directory, normally the category's directory
	Comment  string
}

// Jet is one active connection. M1 only ever has one, but the UI's Jet tab is
// written against a list and M2 fills it in.
type Jet struct {
	Index int   `json:"index"`
	Start int64 `json:"start"`
	End   int64 `json:"end"`
	Done  int64 `json:"done"`
	Speed int64 `json:"speed"`
}

// TaskView is the engine's own row shape. app converts it into its DTOs; keeping
// the two separate is what lets the engine stay free of UI imports.
type TaskView struct {
	ID        string
	Name      string
	URL       string
	Category  string
	State     State
	Comment   string
	Error     string
	Size      int64
	Done      int64
	Speed     int64
	ElapsedMs int64
	RemainMs  int64
	Jets      int
	Blocks    int
	Resumable bool
}

// TaskDetail is the bottom pane's payload.
type TaskDetail struct {
	TaskID string
	Blocks []Block
	Jets   []Jet
	Log    []string
}

// Task is one download.
//
// ID, URL, Category, Comment and Dir are set once at creation and never change,
// so they are read without the lock. Everything else is guarded by mu.
type Task struct {
	ID       string
	URL      string
	Category string
	Comment  string
	Dir      string

	mu        sync.RWMutex
	name      string
	state     State
	size      int64
	done      int64
	speed     int64
	elapsed   time.Duration
	errMsg    string
	resumable bool
	etag      string
	modified  string
	blocks    []Block
	jets      []Jet
	log       []string

	cancel context.CancelFunc
}

func newTask(id string, spec Spec) *Task {
	return &Task{
		ID:       id,
		URL:      spec.URL,
		Category: spec.Category,
		Comment:  spec.Comment,
		Dir:      spec.Dir,
		name:     spec.Name,
		state:    StateDraft,
	}
}

/* ------------------------------------------------------------------- reading */

// State is the current state.
func (t *Task) State() State {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.state
}

// Name is the file name, which is only known after a probe when the caller did
// not supply one.
func (t *Task) Name() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.name
}

// View is a consistent snapshot of the row for the UI.
func (t *Task) View() TaskView {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var remain int64
	if t.size > 0 && t.speed > 0 {
		remain = (t.size - t.done) * 1000 / t.speed
	}

	return TaskView{
		ID:        t.ID,
		Name:      t.name,
		URL:       t.URL,
		Category:  t.Category,
		State:     t.state,
		Comment:   t.Comment,
		Error:     t.errMsg,
		Size:      t.size,
		Done:      t.done,
		Speed:     t.speed,
		ElapsedMs: t.elapsed.Milliseconds(),
		RemainMs:  remain,
		Jets:      len(t.jets),
		Blocks:    len(t.blocks),
		Resumable: t.resumable,
	}
}

// Detail is a snapshot of the block map, jets and log.
func (t *Task) Detail() TaskDetail {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return TaskDetail{
		TaskID: t.ID,
		Blocks: append([]Block(nil), t.blocks...),
		Jets:   append([]Jet(nil), t.jets...),
		Log:    append([]string(nil), t.log...),
	}
}

/* ------------------------------------------------------------------- writing */

func (t *Task) setState(s State) {
	t.mu.Lock()
	from := t.state
	t.state = s
	t.mu.Unlock()

	if from != s {
		t.logf("%s → %s", from, s)
	}
}

func (t *Task) setName(name string) {
	t.mu.Lock()
	t.name = name
	t.mu.Unlock()
}

func (t *Task) setError(err error) {
	t.mu.Lock()
	t.state = StateError
	if err != nil {
		t.errMsg = err.Error()
	}
	t.mu.Unlock()
	if err != nil {
		t.logf("失败：%v", err)
	}
}

func (t *Task) clearError() {
	t.mu.Lock()
	t.errMsg = ""
	t.mu.Unlock()
}

// logf appends a line to the log panel, dropping the oldest when full.
func (t *Task) logf(format string, args ...any) {
	line := time.Now().Format("15:04:05") + " " + fmt.Sprintf(format, args...)

	t.mu.Lock()
	t.log = append(t.log, line)
	if len(t.log) > logLimit {
		t.log = append(t.log[:0], t.log[len(t.log)-logLimit:]...)
	}
	t.mu.Unlock()
}

// prepare resets the plan for a download that starts from byte zero.
func (t *Task) prepare(size int64, etag, modified string, resumable bool, blocks []Block) {
	// A plan carried over from a previous run still has stale claims attached.
	ClearJets(blocks)

	t.mu.Lock()
	t.size = size
	t.etag = etag
	t.modified = modified
	t.resumable = resumable
	t.blocks = blocks
	t.done = TotalDone(blocks)
	t.errMsg = ""
	t.jets = nil
	t.mu.Unlock()
}

// setBlockJet records which connection is writing block i, so the block map can
// show work in progress rather than only the bytes already on disk.
func (t *Task) setBlockJet(i, jet int) {
	t.mu.Lock()
	if i >= 0 && i < len(t.blocks) {
		t.blocks[i].Jet = jet
	}
	t.mu.Unlock()
}

// blockAt copies block i so the runner never holds the lock across I/O.
func (t *Task) blockAt(i int) Block {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if i < 0 || i >= len(t.blocks) {
		return Block{}
	}
	return t.blocks[i]
}

// advance records n newly written bytes for block i, updating the row total.
func (t *Task) advance(i int, n int64) {
	if n <= 0 {
		return
	}
	t.mu.Lock()
	if i >= 0 && i < len(t.blocks) {
		t.blocks[i].Done += n
		if t.blocks[i].Done > t.blocks[i].Len() {
			t.blocks[i].Done = t.blocks[i].Len()
		}
	}
	t.done += n
	t.mu.Unlock()
}

// setJets publishes the active connections for the Jet tab.
func (t *Task) setJets(jets []Jet) {
	t.mu.Lock()
	t.jets = jets
	t.mu.Unlock()
}

func (t *Task) setSpeed(bytesPerSecond int64) {
	t.mu.Lock()
	t.speed = bytesPerSecond
	t.mu.Unlock()
}

// addElapsed accumulates active download time, so a task that was paused for an
// hour does not claim it was downloading for an hour.
func (t *Task) addElapsed(d time.Duration) {
	if d <= 0 {
		return
	}
	t.mu.Lock()
	t.elapsed += d
	t.mu.Unlock()
}

func (t *Task) elapsedNow() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.elapsed
}

// reset throws away all progress and puts the task back to draft. Used by
// Restart, which is the only caller allowed to discard bytes on disk.
func (t *Task) reset() {
	t.mu.Lock()
	t.blocks = nil
	t.done = 0
	t.speed = 0
	t.jets = nil
	t.errMsg = ""
	t.elapsed = 0
	t.state = StateDraft
	t.mu.Unlock()
}

// journal builds the on-disk record from the current state.
func (t *Task) journal() *Journal {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return &Journal{
		URL:       t.URL,
		Name:      t.name,
		Size:      t.size,
		ETag:      t.etag,
		Modified:  t.modified,
		Resumable: t.resumable,
		Blocks:    append([]Block(nil), t.blocks...),
	}
}

// applyJournal rehydrates a task from a state file (process restart).
func (t *Task) applyJournal(j *Journal, blocks []Block) {
	ClearJets(blocks)

	t.mu.Lock()
	t.name = j.Name
	t.size = j.Size
	t.etag = j.ETag
	t.modified = j.Modified
	t.resumable = j.Resumable
	t.blocks = blocks
	t.done = TotalDone(blocks)
	t.state = StatePaused
	t.mu.Unlock()
}
