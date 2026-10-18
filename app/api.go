package app

import (
	"fmt"
	"os/exec"
	"runtime"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/hgta/flashdown/internal/config"
	"github.com/hgta/flashdown/internal/version"
)

// Version is stamped into the startup payload. The constant lives in its own
// package because the CLI needs it too, and must not import this one.
const Version = version.Value

// StartupInfo is everything the frontend needs for its first paint, so it can
// go from mounted to rendered in a single round trip instead of three calls.
type StartupInfo struct {
	Config     config.Config `json:"config"`
	Snapshot   Snapshot      `json:"snapshot"`
	Warning    string        `json:"warning"`
	ConfigPath string        `json:"configPath"`
	Version    string        `json:"version"`
}

// Service is the object bound to the frontend. Every exported method here
// becomes a typed TS function under frontend/bindings.
type Service struct {
	mu      sync.RWMutex
	cfg     config.Config
	warning string

	app   *application.App
	main  *application.WebviewWindow
	float *application.WebviewWindow
	hub   *Hub

	// OnConfigSaved lets the shell hand new settings to the download engine. It is
	// a plain field rather than a method so the binding generator never sees it.
	OnConfigSaved func(config.Config)

	// Tasks is the engine's write side, injected by main. Nil means "no engine"
	// (demo mode), and every control then refuses with ErrNoEngine instead of
	// pretending to work.
	Tasks TaskController
}

// NewService creates the bound service with the already-loaded configuration.
func NewService(cfg config.Config, warning string) *Service {
	return &Service{cfg: cfg, warning: warning}
}

// Attach hands the service the runtime objects it needs, then wires the things
// that can only exist once those objects do: the floating window's native
// context menu and the native file-drop bridge. Called from main after the
// windows exist.
func (s *Service) Attach(wailsApp *application.App, main, float *application.WebviewWindow, hub *Hub) {
	s.app = wailsApp
	s.main = main
	s.float = float
	s.hub = hub

	s.registerFloatMenu()
	s.forwardFileDrops()
}

// GetStartupInfo returns configuration plus the current snapshot for first paint.
func (s *Service) GetStartupInfo() StartupInfo {
	path, _ := config.Path()

	s.mu.RLock()
	cfg, warning := s.cfg, s.warning
	s.mu.RUnlock()

	snap := Snapshot{}
	if s.hub != nil {
		snap = s.hub.Current()
	}

	return StartupInfo{
		Config:     cfg,
		Snapshot:   snap,
		Warning:    warning,
		ConfigPath: path,
		Version:    Version,
	}
}

// GetConfig returns the live configuration.
func (s *Service) GetConfig() config.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// SaveConfig persists the configuration and returns the normalized value so the
// frontend's copy never drifts from disk.
func (s *Service) SaveConfig(cfg config.Config) (config.Config, error) {
	cfg.Normalize()

	if err := config.Save(cfg); err != nil {
		return s.GetConfig(), err
	}
	// Category directories may not exist yet; recreate them eagerly so the
	// "open folder" action never fails on a fresh install.
	if err := config.EnsureDirectories(cfg); err != nil {
		return cfg, err
	}

	s.mu.Lock()
	s.cfg = cfg
	s.warning = ""
	s.mu.Unlock()

	// Jet count, retry policy and limits only take effect for runs that start
	// after this point - which is the honest behaviour for a running download.
	if s.OnConfigSaved != nil {
		s.OnConfigSaved(cfg)
	}

	return cfg, nil
}

// SaveLayout persists only the splitter ratios, so dragging a divider doesn't
// rewrite the whole settings file dozens of times per second.
func (s *Service) SaveLayout(treeWidthRatio, detailHeightRatio float64) error {
	s.mu.Lock()
	s.cfg.UI.TreeWidthRatio = treeWidthRatio
	s.cfg.UI.DetailHeightRatio = detailHeightRatio
	cfg := s.cfg
	s.mu.Unlock()

	cfg.Normalize()
	return config.Save(cfg)
}

// SelectTask tells the hub which task the bottom pane should follow.
func (s *Service) SelectTask(id string) {
	if s.hub != nil {
		s.hub.SelectTask(id)
	}
}

// SnapshotNow returns the latest cached snapshot on demand.
func (s *Service) SnapshotNow() Snapshot {
	if s.hub == nil {
		return Snapshot{}
	}
	return s.hub.Current()
}

// OpenFolder reveals a directory in the OS file manager.
func (s *Service) OpenFolder(dir string) error {
	if dir == "" {
		return fmt.Errorf("empty directory")
	}
	switch runtime.GOOS {
	case "windows":
		// explorer.exe returns a non-zero exit code even on success, so the
		// error is intentionally discarded.
		_ = exec.Command("explorer", dir).Start()
		return nil
	case "darwin":
		return exec.Command("open", dir).Start()
	default:
		return exec.Command("xdg-open", dir).Start()
	}
}

// ShowMainWindow restores and focuses the main window.
func (s *Service) ShowMainWindow() {
	if s.main != nil {
		s.main.Show()
	}
}
