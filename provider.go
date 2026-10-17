package main

import (
	"github.com/hgta/flashdown/app"
	"github.com/hgta/flashdown/internal/engine"
)

// engineProvider adapts the download engine to app's two seams: Provider (read)
// and TaskController (write).
//
// The adapter lives in main on purpose. app must not import the engine, because
// the engine has to stay runnable without a window; the engine must not import
// app, because that would drag Wails into every engine test. main is the only
// package allowed to know both, which is exactly what "injected" means here.
type engineProvider struct {
	m *engine.Manager
}

// Add implements app.TaskController.
func (p engineProvider) Add(req app.AddTaskRequest) (string, error) {
	t, err := p.m.Add(engine.Spec{
		URL:      req.URL,
		Name:     req.Name,
		Dir:      req.Dir,
		Category: req.Category,
		Comment:  req.Comment,
	})
	if err != nil {
		return "", err
	}
	if req.Start {
		if err := p.m.Start(t.ID); err != nil {
			return t.ID, err
		}
	}
	return t.ID, nil
}

func (p engineProvider) Start(id string) error  { return p.m.Start(id) }
func (p engineProvider) Pause(id string) error  { return p.m.Pause(id) }
func (p engineProvider) StartAll()              { p.m.StartAll() }
func (p engineProvider) PauseAll()              { p.m.PauseAll() }
func (p engineProvider) Restart(id string) error { return p.m.Restart(id) }

func (p engineProvider) SetMaxJets(n int) int { return p.m.SetMaxJets(n) }

func (p engineProvider) Remove(id string, deleteFile bool) error {
	return p.m.Remove(id, deleteFile)
}

func (p engineProvider) Snapshot() app.Snapshot {
	src := p.m.Snapshot()

	out := app.Snapshot{
		Tasks:       make([]app.TaskDTO, 0, len(src.Tasks)),
		Downloading: src.Downloading,
		TotalSpeed:  src.TotalSpeed,
		ActiveJets:  src.ActiveJets,
		AtMillis:    src.AtMillis,
	}
	for _, t := range src.Tasks {
		out.Tasks = append(out.Tasks, app.TaskDTO{
			ID:        t.ID,
			Name:      t.Name,
			URL:       t.URL,
			Category:  t.Category,
			State:     string(t.State),
			Size:      t.Size,
			Done:      t.Done,
			Speed:     t.Speed,
			ElapsedMs: t.ElapsedMs,
			RemainMs:  t.RemainMs,
			Jets:      t.Jets,
			Blocks:    t.Blocks,
			Resumable: t.Resumable,
			Comment:   t.Comment,
			Error:     t.Error,
		})
	}
	return out
}

func (p engineProvider) Detail(taskID string) app.TaskDetail {
	src := p.m.Detail(taskID)

	out := app.TaskDetail{
		TaskID: src.TaskID,
		Blocks: make([]app.BlockDTO, 0, len(src.Blocks)),
		Jets:   make([]app.JetDTO, 0, len(src.Jets)),
		Log:    src.Log,
	}
	for _, b := range src.Blocks {
		out.Blocks = append(out.Blocks, app.BlockDTO{
			Start: b.Start,
			End:   b.End,
			Done:  b.Done,
			Jet:   b.Jet,
		})
	}
	for _, j := range src.Jets {
		out.Jets = append(out.Jets, app.JetDTO{
			Index: j.Index,
			Start: j.Start,
			End:   j.End,
			Done:  j.Done,
			Speed: j.Speed,
		})
	}
	return out
}
