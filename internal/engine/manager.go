package engine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hgta/flashdown/internal/config"
)

// Snapshot is the engine's view of the whole grid. app converts it into the DTOs
// the UI already speaks.
type Snapshot struct {
	Tasks       []TaskView
	Downloading int
	TotalSpeed  int64
	ActiveJets  int
	AtMillis    int64
}

// Manager owns the task list and the goroutines that drive it.
type Manager struct {
	cfgMu sync.RWMutex
	cfg   config.Config

	client *http.Client

	// gate caps how many tasks may be downloading at once (3.7). Blocking on it
	// is what makes the waiting state mean something.
	gate chan struct{}
	wg   sync.WaitGroup

	mu    sync.RWMutex
	tasks []*Task
	byID  map[string]*Task
	seq   uint64
}

// New creates a manager. The engine never reads config from disk itself - the
// caller owns the settings file, so tests can pass a bare struct.
func New(cfg config.Config) *Manager {
	m := &Manager{
		client: NewClient(),
		byID:   make(map[string]*Task),
		gate:   make(chan struct{}, maxConcurrent(cfg)),
	}
	m.cfg = cfg
	return m
}

func maxConcurrent(cfg config.Config) int {
	if cfg.MaxConcurrent < 1 {
		return 1
	}
	return cfg.MaxConcurrent
}

// ApplyConfig picks up new settings.
//
// The gate is deliberately not resized: shrinking it would strand slots that are
// already held, and a real scheduler replaces this gate in §3.7 anyway.
func (m *Manager) ApplyConfig(cfg config.Config) {
	m.cfgMu.Lock()
	m.cfg = cfg
	m.cfgMu.Unlock()
}

// SetMaxJets changes how many connections a download may use and returns the
// value actually stored. Running tasks pick it up at their next scheduling tick,
// so raising or lowering the count never interrupts a request in flight (3.4).
func (m *Manager) SetMaxJets(n int) int {
	if n < 1 {
		n = 1
	}
	if n > maxJetsCeiling {
		n = maxJetsCeiling
	}
	m.cfgMu.Lock()
	m.cfg.MaxJets = n
	m.cfgMu.Unlock()
	return n
}

// MaxJets is the connection count currently in effect.
func (m *Manager) MaxJets() int {
	return m.desiredJets(0)
}

// Config returns a copy of the settings a run should use.
func (m *Manager) Config() config.Config {
	m.cfgMu.RLock()
	defer m.cfgMu.RUnlock()
	return m.cfg
}

// acquireSlot blocks until a download slot is free. Blocking here, inside the
// task's own goroutine, is what gives the waiting state its meaning: the task is
// queued rather than competing for bandwidth (3.7).
func (m *Manager) acquireSlot(ctx context.Context) error {
	select {
	case m.gate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) releaseSlot() {
	select {
	case <-m.gate:
	default:
	}
}

/* ------------------------------------------------------------------- lifecycle */

// Add registers a task without starting it: the new-task dialog offers an
// "开始方式", and the caller decides (2.13).
func (m *Manager) Add(spec Spec) (*Task, error) {
	u, err := url.Parse(strings.TrimSpace(spec.URL))
	if err != nil {
		return nil, fmt.Errorf("链接无效：%w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("只支持 http / https 链接")
	}
	if u.Host == "" {
		return nil, errors.New("链接缺少主机名")
	}
	if strings.TrimSpace(spec.Dir) == "" {
		return nil, errors.New("未指定保存目录")
	}

	spec.URL = u.String()
	// A name typed into the dialog wins; otherwise the probe fills it in later.
	spec.Name = sanitizeName(spec.Name)

	m.mu.Lock()
	m.seq++
	t := newTask(fmt.Sprintf("t%d", m.seq), spec)
	m.tasks = append(m.tasks, t)
	m.byID[t.ID] = t
	m.mu.Unlock()

	t.logf("已创建任务：%s", spec.URL)
	return t, nil
}

// Start begins, or resumes, a download on its own goroutine.
func (m *Manager) Start(id string) error {
	t := m.Task(id)
	if t == nil {
		return fmt.Errorf("任务不存在：%s", id)
	}

	switch t.State() {
	case StateDownloading, StateProbing, StateWaiting, StateVerifying, StateCompleting:
		return nil // already on its way
	case StateRemoved:
		return errors.New("任务已删除，请先还原")
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.clearError()
	t.logf("开始下载")

	t.mu.Lock()
	prev := t.cancel
	t.cancel = cancel
	t.mu.Unlock()
	if prev != nil {
		// A second Start while a run is winding down: let the old one finish so
		// two runs can never write the same file at once.
		prev()
	}

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer cancel()
		m.download(ctx, t)
	}()
	return nil
}

// Pause asks a running download to stop. The goroutine persists its progress
// before it exits, so this returns before the bytes are necessarily on disk.
func (m *Manager) Pause(id string) error {
	t := m.Task(id)
	if t == nil {
		return fmt.Errorf("任务不存在：%s", id)
	}

	t.mu.Lock()
	cancel := t.cancel
	t.cancel = nil
	t.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	return nil
}

// StartAll starts every task that is not already running or finished.
func (m *Manager) StartAll() {
	for _, t := range m.Tasks() {
		switch t.State() {
		case StateFinished, StateRemoved:
			continue
		}
		_ = m.Start(t.ID)
	}
}

// PauseAll pauses everything that is running.
func (m *Manager) PauseAll() {
	for _, t := range m.Tasks() {
		_ = m.Pause(t.ID)
	}
}

// Restart discards the saved progress and downloads from byte zero. This is the
// "重新下载" half of 2.9's choice, and the only way out of ErrRemoteChanged.
func (m *Manager) Restart(id string) error {
	t := m.Task(id)
	if t == nil {
		return fmt.Errorf("任务不存在：%s", id)
	}

	_ = m.Pause(id)
	dataPath, journalPath := Paths(t.Dir, t.Name())
	_ = RemoveJournal(journalPath)
	_ = os.Remove(dataPath)

	t.reset()
	t.logf("已清除原有进度")
	return m.Start(id)
}

// Remove moves a task to the removed node. The row stays in the list so the
// 已删除 node can show it and it can be brought back (4.11).
func (m *Manager) Remove(id string, deleteData bool) error {
	t := m.Task(id)
	if t == nil {
		return fmt.Errorf("任务不存在：%s", id)
	}

	_ = m.Pause(id)
	t.setState(StateRemoved)
	t.setJets(nil)
	t.setSpeed(0)

	if deleteData {
		dataPath, journalPath := Paths(t.Dir, t.Name())
		_ = RemoveJournal(journalPath)
		if err := os.Remove(dataPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("删除文件失败：%w", err)
		}
	}
	return nil
}

// Undelete brings a removed task back as a paused row.
func (m *Manager) Undelete(id string) error {
	t := m.Task(id)
	if t == nil {
		return fmt.Errorf("任务不存在：%s", id)
	}
	if t.State() != StateRemoved {
		return nil
	}
	t.setState(StatePaused)
	return nil
}

// Shutdown pauses everything and waits for the goroutines to flush, so quitting
// never loses the last second of progress (5.7).
func (m *Manager) Shutdown(ctx context.Context) {
	m.PauseAll()

	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
	}
}

/* ---------------------------------------------------------------------- reading */

// Task looks a task up by id.
func (m *Manager) Task(id string) *Task {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.byID[id]
}

// Tasks returns the current list in insertion order.
func (m *Manager) Tasks() []*Task {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]*Task(nil), m.tasks...)
}

// Snapshot builds the whole grid. Task views are copied so the caller never
// holds a lock while rendering.
func (m *Manager) Snapshot() Snapshot {
	tasks := m.Tasks()

	out := Snapshot{
		Tasks:    make([]TaskView, 0, len(tasks)),
		AtMillis: time.Now().UnixMilli(),
	}
	for _, t := range tasks {
		v := t.View()
		out.Tasks = append(out.Tasks, v)
		switch v.State {
		case StateDownloading, StateProbing:
			out.Downloading++
			out.TotalSpeed += v.Speed
			out.ActiveJets += v.Jets
		}
	}
	return out
}

// Detail returns the block map, jets and log for one task.
func (m *Manager) Detail(id string) TaskDetail {
	t := m.Task(id)
	if t == nil {
		return TaskDetail{TaskID: id}
	}
	return t.Detail()
}

/* ---------------------------------------------------------------------- restore */

// Restore rebuilds the task list from the journals on disk. This is what makes a
// download survive a crash or a reboot: the data file is there, and the journal
// says how much of it is trustworthy (2.7 / 6.1).
func (m *Manager) Restore() error {
	cfg := m.Config()

	dirs := make([]string, 0, 1+len(cfg.Categories))
	dirs = append(dirs, cfg.DownloadRoot)
	for _, c := range cfg.Categories {
		dirs = append(dirs, c.Dir)
	}

	seen := make(map[string]bool, len(dirs))
	for _, dir := range dirs {
		if dir == "" || seen[dir] {
			continue
		}
		seen[dir] = true

		// A missing category directory is normal - the user may have deleted it.
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		category := ""
		for _, c := range cfg.Categories {
			if c.Dir == dir {
				category = c.ID
			}
		}

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), DataSuffix+JournalSuffix) {
				continue
			}
			j, err := LoadJournal(filepath.Join(dir, entry.Name()))
			if err != nil {
				// Corrupt or already backed up: the data file alone cannot be
				// trusted, so it is left for the user to inspect.
				continue
			}
			if len(j.Blocks) == 0 {
				continue
			}
			m.adopt(j, dir, category)
		}
	}
	return nil
}

// adopt registers a restored journal as a paused task.
func (m *Manager) adopt(j *Journal, dir, category string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, t := range m.tasks {
		if t.URL == j.URL && t.Dir == dir {
			return
		}
	}

	m.seq++
	t := newTask(fmt.Sprintf("t%d", m.seq), Spec{
		URL:      j.URL,
		Name:     j.Name,
		Category: category,
		Dir:      dir,
	})
	// The journal may be one flush behind the data file. That direction is safe:
	// the missing bytes are simply fetched again. The reverse would not be.
	t.applyJournal(j, j.Blocks)

	m.tasks = append(m.tasks, t)
	m.byID[t.ID] = t
	t.logf("从磁盘恢复未完成任务：%s（%d/%d 字节）", j.Name, j.Done(), j.Size)
}
