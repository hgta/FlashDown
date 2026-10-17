package app

import (
	"errors"
	"strings"
)

// AddTaskRequest is what the new-task dialog sends.
//
// A flat struct with json tags rather than an engine type: app must not import the
// engine, so this is the whole contract between the two layers (2.13).
type AddTaskRequest struct {
	URL      string `json:"url"`
	Name     string `json:"name"`
	Dir      string `json:"dir"`
	Category string `json:"category"`
	Comment  string `json:"comment"`
	Start    bool   `json:"start"`
}

// TaskController is the write side of the engine seam.
//
// Provider is the read side. Keeping them apart is what lets --demo stay
// read-only: the grid can be exercised with fabricated data while the controls
// honestly refuse to act on tasks that do not exist.
type TaskController interface {
	Add(req AddTaskRequest) (id string, err error)
	Start(id string) error
	Pause(id string) error
	StartAll()
	PauseAll()
	Restart(id string) error
	Remove(id string, deleteFile bool) error
}

// ErrNoEngine explains a control that has nothing to act on.
var ErrNoEngine = errors.New("当前没有可用的下载引擎（演示模式或引擎未注入）")

func (s *Service) controller() (TaskController, error) {
	if s.Tasks == nil {
		return nil, ErrNoEngine
	}
	return s.Tasks, nil
}

// AddTask queues a download and optionally starts it.
func (s *Service) AddTask(req AddTaskRequest) (string, error) {
	ctrl, err := s.controller()
	if err != nil {
		return "", err
	}

	req.URL = strings.TrimSpace(req.URL)
	if req.URL == "" {
		return "", errors.New("请填写下载链接")
	}
	req.Name = strings.TrimSpace(req.Name)

	// An empty directory means "use the category's directory", which is what the
	// dialog defaults to and what dropping a link in a hurry relies on.
	if strings.TrimSpace(req.Dir) == "" {
		req.Dir = s.defaultDir(req.Category)
	}

	return ctrl.Add(req)
}

// defaultDir resolves a category id to its directory, falling back to the global
// download root.
func (s *Service) defaultDir(category string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, c := range s.cfg.Categories {
		if c.ID == category && c.Dir != "" {
			return c.Dir
		}
	}
	return s.cfg.DownloadRoot
}

// StartTask begins or resumes one task.
func (s *Service) StartTask(id string) error {
	ctrl, err := s.controller()
	if err != nil {
		return err
	}
	return ctrl.Start(id)
}

// PauseTask asks one task to stop and keeps its progress.
func (s *Service) PauseTask(id string) error {
	ctrl, err := s.controller()
	if err != nil {
		return err
	}
	return ctrl.Pause(id)
}

// StartAllTasks starts everything that is not finished or removed.
func (s *Service) StartAllTasks() error {
	ctrl, err := s.controller()
	if err != nil {
		return err
	}
	ctrl.StartAll()
	return nil
}

// PauseAllTasks pauses everything that is running.
func (s *Service) PauseAllTasks() error {
	ctrl, err := s.controller()
	if err != nil {
		return err
	}
	ctrl.PauseAll()
	return nil
}

// RestartTask throws away the saved progress and downloads from byte zero - the
// "重新下载" side of the choice §2.9 leaves to the user.
func (s *Service) RestartTask(id string) error {
	ctrl, err := s.controller()
	if err != nil {
		return err
	}
	return ctrl.Restart(id)
}

// RemoveTask moves a task to the removed node; deleteFile also deletes the
// partial data and its journal.
func (s *Service) RemoveTask(id string, deleteFile bool) error {
	ctrl, err := s.controller()
	if err != nil {
		return err
	}
	return ctrl.Remove(id, deleteFile)
}

// PickFolder opens the OS directory chooser. Asking a user to type a Windows path
// by hand is not a reasonable thing to do, so the dialog needs this.
//
// An empty result with a nil error means the user cancelled.
func (s *Service) PickFolder(startAt string) (string, error) {
	if s.app == nil {
		return "", errors.New("窗口尚未就绪")
	}

	dialog := s.app.Dialog.OpenFile().
		CanChooseFiles(false).
		CanChooseDirectories(true).
		CanCreateDirectories(true).
		SetTitle("选择保存目录")
	if strings.TrimSpace(startAt) != "" {
		dialog.SetDirectory(startAt)
	}
	return dialog.PromptForSingleSelection()
}

// OpenNewTask brings the main window forward and asks it to open the new-task
// dialog, optionally pre-filled with a link.
//
// This is how a drop on the floating window becomes a dialog in the main window:
// the two have separate JS contexts, so the request has to travel through Go.
func (s *Service) OpenNewTask(payload string) error {
	if s.app == nil {
		return errors.New("窗口尚未就绪")
	}

	if s.main != nil {
		s.main.Show()
		s.main.Focus()
	}
	s.app.Event.Emit(EventNewTask, NewTask{URL: strings.TrimSpace(payload)})
	return nil
}

// NewTask is the payload of EventNewTask.
type NewTask struct {
	URL string `json:"url"`
}


