// Package app is the Wails-facing layer: it exposes Go services to the frontend
// and broadcasts engine progress on a throttled schedule (design.md D7).
//
// Nothing here may import the engine directly - the engine implements Provider
// and is injected, which keeps it headless and unit-testable.
package app

import "time"

// Event names. A single emit reaches every window, which is what lets the
// floating window reuse the main window's speed data for free.
// Verified by Spike A: 22/22 emits acked by both windows, zero loss.
const (
	EventTasksSnapshot = "tasks:snapshot"
	EventTaskDetail    = "task:detail"
	EventSpeedTick     = "speed:tick"
	// EventFilesDropped carries OS file drops, which the DOM never sees.
	EventFilesDropped = "files:dropped"
	// EventNewTask asks the main window to open the new-task dialog.
	EventNewTask = "dialog:new-task"
)

// Throttling intervals from design.md D7. The engine must never push per-byte
// or per-jet updates at the UI - that is what floods WebView2.
const (
	SnapshotInterval = 500 * time.Millisecond
	DetailInterval   = 250 * time.Millisecond
	SpeedInterval    = time.Second
	SpeedHistoryLen  = 60
)

// Task states, mirroring the engine state machine in design.md.
const (
	StateDraft       = "draft"
	StateWaiting     = "waiting"
	StateProbing     = "probing"
	StateDownloading = "downloading"
	StatePaused      = "paused"
	StateVerifying   = "verifying"
	StateCompleting  = "completing"
	StateFinished    = "finished"
	StateError       = "error"
	StateRemoved     = "removed"
)

// TaskDTO is one row of the task grid.
type TaskDTO struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	Category  string `json:"category"`
	State     string `json:"state"`
	Size      int64  `json:"size"`
	Done      int64  `json:"done"`
	Speed     int64  `json:"speed"`
	ElapsedMs int64  `json:"elapsedMs"`
	RemainMs  int64  `json:"remainMs"`
	Jets      int    `json:"jets"`
	Blocks    int    `json:"blocks"`
	Resumable bool   `json:"resumable"`
	Comment   string `json:"comment"`
	Error     string `json:"error"`
}

// Snapshot is the whole grid. The frontend replaces its list wholesale rather
// than diffing, which keeps the render path trivial at 2 fps.
type Snapshot struct {
	Tasks       []TaskDTO `json:"tasks"`
	Downloading int       `json:"downloading"`
	TotalSpeed  int64     `json:"totalSpeed"`
	ActiveJets  int       `json:"activeJets"`
	AtMillis    int64     `json:"atMillis"`
}

// BlockDTO feeds the 块图 canvas. Jet is -1 when the block is unclaimed.
type BlockDTO struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
	Done  int64 `json:"done"`
	Jet   int   `json:"jet"`
}

// JetDTO feeds the Jet tab.
type JetDTO struct {
	Index int   `json:"index"`
	Start int64 `json:"start"`
	End   int64 `json:"end"`
	Done  int64 `json:"done"`
	Speed int64 `json:"speed"`
}

// TaskDetail is the bottom pane's payload for the currently selected task.
type TaskDetail struct {
	TaskID string     `json:"taskId"`
	Blocks []BlockDTO `json:"blocks"`
	Jets   []JetDTO   `json:"jets"`
	Log    []string   `json:"log"`
}

// SpeedTick drives the floating window's graph and the status bar.
type SpeedTick struct {
	Speed    int64   `json:"speed"`
	History  []int64 `json:"history"`
	AtMillis int64   `json:"atMillis"`
}

// FilesDropped is the payload of EventFilesDropped. It carries filesystem
// paths, not URLs - an OS drop hands over paths, and the new-task dialog (M1
// §2.13) is what decides whether a given path is worth queueing.
type FilesDropped struct {
	Files []string `json:"files"`
}

// Provider is the seam between the Wails layer and the download engine.
// Implementations must return cached state cheaply - these run several times a
// second.
type Provider interface {
	Snapshot() Snapshot
	Detail(taskID string) TaskDetail
}
