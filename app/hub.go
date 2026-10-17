package app

import (
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Hub turns engine state into the three throttled broadcast events. It is the
// only place in the program that calls Event.Emit.
type Hub struct {
	app      *application.App
	provider Provider

	mu       sync.RWMutex
	selected string
	cached   Snapshot
	history  []int64

	stopOnce sync.Once
	stopped  chan struct{}
}

// NewHub wires a provider to the application's event bus.
func NewHub(app *application.App, provider Provider) *Hub {
	return &Hub{
		app:      app,
		provider: provider,
		history:  make([]int64, 0, SpeedHistoryLen),
		stopped:  make(chan struct{}),
	}
}

// Start emits an initial pair of events and then runs the three tickers.
func (h *Hub) Start() {
	h.refreshAndEmitSnapshot()
	h.emitSpeed()

	go h.loop(SnapshotInterval, h.refreshAndEmitSnapshot)
	go h.loop(DetailInterval, h.emitDetail)
	go h.loop(SpeedInterval, h.emitSpeed)
}

// Stop halts every ticker. Safe to call more than once.
func (h *Hub) Stop() {
	h.stopOnce.Do(func() { close(h.stopped) })
}

func (h *Hub) loop(every time.Duration, fn func()) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-h.stopped:
			return
		case <-t.C:
			fn()
		}
	}
}

// SelectTask switches which task the bottom pane follows. An empty id stops
// detail emission entirely.
func (h *Hub) SelectTask(id string) {
	h.mu.Lock()
	changed := h.selected != id
	h.selected = id
	h.mu.Unlock()

	if changed {
		h.emitDetail()
	}
}

// Current returns the most recent snapshot. The frontend uses this for its
// first paint instead of waiting up to 500ms for a tick.
func (h *Hub) Current() Snapshot {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.cached
}

// refreshAndEmitSnapshot is the single producer of Snapshot values; emitSpeed
// reuses its cache so the two events can never disagree.
func (h *Hub) refreshAndEmitSnapshot() {
	snap := h.provider.Snapshot()
	snap.AtMillis = time.Now().UnixMilli()

	h.mu.Lock()
	h.cached = snap
	h.mu.Unlock()

	h.app.Event.Emit(EventTasksSnapshot, snap)
}

func (h *Hub) emitDetail() {
	h.mu.RLock()
	id := h.selected
	h.mu.RUnlock()

	if id == "" {
		return
	}
	h.app.Event.Emit(EventTaskDetail, h.provider.Detail(id))
}

func (h *Hub) emitSpeed() {
	h.mu.Lock()
	speed := h.cached.TotalSpeed
	h.history = append(h.history, speed)
	if len(h.history) > SpeedHistoryLen {
		h.history = h.history[len(h.history)-SpeedHistoryLen:]
	}
	history := make([]int64, len(h.history))
	copy(history, h.history)
	h.mu.Unlock()

	h.app.Event.Emit(EventSpeedTick, SpeedTick{
		Speed:    speed,
		History:  history,
		AtMillis: time.Now().UnixMilli(),
	})
}
