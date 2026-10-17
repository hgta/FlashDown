// Package config owns FlashDown's user settings: where downloads land, the
// category tree, download rules and the engine tunables.
//
// Everything lives in one JSON document at %APPDATA%\FlashDown\config.json.
// It is written atomically (temp file -> fsync -> rename) and a corrupt file is
// backed up rather than crashing the app.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	appDirName = "FlashDown"
	configName = "config.json"

	// Preallocation policies. "auto" reserves disk space only when the volume
	// can comfortably afford it - see design.md D4: Truncate costs *space*, not
	// time, so the deciding factor is free bytes, not filesystem speed.
	PreallocateAuto   = "auto"
	PreallocateAlways = "always"
	PreallocateNever  = "never"

	ThemeSystem = "system"
	ThemeLight  = "light"
	ThemeDark   = "dark"

	// CurrentVersion is bumped whenever the on-disk shape changes.
	CurrentVersion = 1
)

// Category is a user-visible bucket with its own target directory.
type Category struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Dir       string `json:"dir"`
	FileTypes string `json:"fileTypes"`
	BuiltIn   bool   `json:"builtIn"`
}

// Rule maps file-name patterns onto a category. First match wins.
type Rule struct {
	Patterns string `json:"patterns"`
	Category string `json:"category"`
	Enabled  bool   `json:"enabled"`
}

// Retry controls how stubbornly a failing task keeps trying.
type Retry struct {
	MaxAttempts    int `json:"maxAttempts"`
	BackoffSeconds int `json:"backoffSeconds"`
}

// Limits holds the speed ceilings (0 means unlimited).
type Limits struct {
	GlobalBytesPerSec   int64  `json:"globalBytesPerSec"`
	ScheduleEnabled     bool   `json:"scheduleEnabled"`
	ScheduleFrom        string `json:"scheduleFrom"`
	ScheduleTo          string `json:"scheduleTo"`
	ScheduleBytesPerSec int64  `json:"scheduleBytesPerSec"`
}

// UI holds presentation preferences. The splitter positions are stored as
// ratios rather than pixels so they survive a window resize, and they are
// written through SaveLayout so a drag does not rewrite the whole file.
type UI struct {
	Theme             string   `json:"theme"`
	FloatWindow       bool     `json:"floatWindow"`
	NotifyOnFinish    bool     `json:"notifyOnFinish"`
	ClipboardWatch    bool     `json:"clipboardWatch"`
	ClipboardAuto     bool     `json:"clipboardAutoAdd"`
	Columns           []string `json:"columns"`
	TreeWidthRatio    float64  `json:"treeWidthRatio"`
	DetailHeightRatio float64  `json:"detailHeightRatio"`
}

// Config is the whole settings document.
type Config struct {
	Version       int        `json:"version"`
	DownloadRoot  string     `json:"downloadRoot"`
	MaxConcurrent int        `json:"maxConcurrentTasks"`
	MaxJets       int        `json:"maxJets"`
	Preallocate   string     `json:"preallocate"`
	Categories    []Category `json:"categories"`
	Rules         []Rule     `json:"rules"`
	Retry         Retry      `json:"retry"`
	Limits        Limits     `json:"limits"`
	UI            UI         `json:"ui"`
}

// DefaultColumns mirrors FlashGet's grid order. The frontend maps these ids to
// headers, widths and renderers.
var DefaultColumns = []string{
	"name", "size", "progress", "elapsed", "remaining", "speed", "blocks",
}

// Default returns the settings a first-time user gets.
func Default() Config {
	root := DefaultDownloadRoot()

	mk := func(id, name, types string) Category {
		return Category{
			ID:        id,
			Name:      name,
			Dir:       filepath.Join(root, name),
			FileTypes: types,
			BuiltIn:   true,
		}
	}

	return Config{
		Version:       CurrentVersion,
		DownloadRoot:  root,
		MaxConcurrent: 3,
		MaxJets:       5,
		Preallocate:   PreallocateAuto,
		Categories: []Category{
			mk("software", "软件", "*.zip;*.rar;*.7z;*.exe;*.msi"),
			mk("game", "游戏", "*.iso;*.img;*.bin"),
			mk("driver", "驱动程序", "*.inf;*.sys;*.cab"),
			mk("mp3", "MP3", "*.mp3;*.flac;*.wav;*.m4a"),
		},
		Rules: []Rule{
			{Patterns: "*.zip;*.rar;*.7z;*.exe;*.msi", Category: "software", Enabled: true},
			{Patterns: "*.iso;*.img;*.bin", Category: "game", Enabled: true},
			{Patterns: "*.inf;*.sys;*.cab", Category: "driver", Enabled: true},
			{Patterns: "*.mp3;*.flac;*.wav;*.m4a", Category: "mp3", Enabled: true},
		},
		Retry: Retry{MaxAttempts: 5, BackoffSeconds: 10},
		Limits: Limits{
			GlobalBytesPerSec: 0,
			ScheduleFrom:      "09:00",
			ScheduleTo:        "18:00",
		},
		UI: UI{
			Theme:             ThemeSystem,
			FloatWindow:       true,
			NotifyOnFinish:    true,
			ClipboardWatch:    false,
			Columns:           append([]string(nil), DefaultColumns...),
			TreeWidthRatio:    0.20,
			DetailHeightRatio: 0.32,
		},
	}
}

// DefaultDownloadRoot is <user>\Downloads\FlashDown on Windows.
func DefaultDownloadRoot() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, "Downloads", appDirName)
	}
	return filepath.Join(os.TempDir(), appDirName)
}

// AppDir is %APPDATA%\FlashDown (os.UserConfigDir is %AppData% on Windows).
func AppDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(base, appDirName), nil
}

// Path is the full path of the settings file.
func Path() (string, error) {
	dir, err := AppDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configName), nil
}

// Load reads the settings file. A missing file yields defaults with no warning.
// A corrupt file is moved aside and defaults are returned together with a
// human-readable warning, so the app always starts.
func Load() (cfg Config, warning string, err error) {
	path, err := Path()
	if err != nil {
		return Default(), "", err
	}

	raw, readErr := os.ReadFile(path)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			cfg = Default()
			cfg.Normalize()
			return cfg, "", nil
		}
		return Default(), "", fmt.Errorf("read %s: %w", path, readErr)
	}

	if jsonErr := json.Unmarshal(raw, &cfg); jsonErr != nil {
		backup := path + ".bak"
		_ = os.Rename(path, backup)
		cfg = Default()
		cfg.Normalize()
		return cfg, fmt.Sprintf("配置文件损坏，已备份为 %s 并恢复默认设置", filepath.Base(backup)), nil
	}

	cfg.Normalize()
	return cfg, "", nil
}

// Save writes the config atomically: a full temp file, fsynced, then renamed
// over the real path so a crash mid-write can never leave a half-written file.
func Save(cfg Config) error {
	dir, err := AppDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	cfg.Normalize()
	buf, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	buf = append(buf, '\n')

	path := filepath.Join(dir, configName)
	tmp := path + ".tmp"

	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("create %s: %w", tmp, err)
	}
	if _, err := f.Write(buf); err != nil {
		f.Close()
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("sync %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

// Normalize repairs values that are out of range or missing, so the rest of the
// program can trust the struct without re-checking everywhere.
func (c *Config) Normalize() {
	if c.Version == 0 {
		c.Version = CurrentVersion
	}
	if strings.TrimSpace(c.DownloadRoot) == "" {
		c.DownloadRoot = DefaultDownloadRoot()
	}
	if c.MaxConcurrent < 1 {
		c.MaxConcurrent = 3
	}
	if c.MaxConcurrent > 30 {
		c.MaxConcurrent = 30
	}
	// FlashGet's registry trick capped jets at 30; we keep the same ceiling.
	if c.MaxJets < 1 {
		c.MaxJets = 5
	}
	if c.MaxJets > 30 {
		c.MaxJets = 30
	}
	switch c.Preallocate {
	case PreallocateAuto, PreallocateAlways, PreallocateNever:
	default:
		c.Preallocate = PreallocateAuto
	}
	switch c.UI.Theme {
	case ThemeSystem, ThemeLight, ThemeDark:
	default:
		c.UI.Theme = ThemeSystem
	}
	if len(c.UI.Columns) == 0 {
		c.UI.Columns = append([]string(nil), DefaultColumns...)
	}
	if c.UI.TreeWidthRatio < 0.08 || c.UI.TreeWidthRatio > 0.6 {
		c.UI.TreeWidthRatio = 0.20
	}
	if c.UI.DetailHeightRatio < 0.1 || c.UI.DetailHeightRatio > 0.8 {
		c.UI.DetailHeightRatio = 0.32
	}
	if c.Retry.MaxAttempts < 1 {
		c.Retry.MaxAttempts = 5
	}
	if c.Retry.BackoffSeconds < 1 {
		c.Retry.BackoffSeconds = 10
	}
	if c.Limits.ScheduleFrom == "" {
		c.Limits.ScheduleFrom = "09:00"
	}
	if c.Limits.ScheduleTo == "" {
		c.Limits.ScheduleTo = "18:00"
	}
	if c.Categories == nil {
		c.Categories = Default().Categories
	}
	if c.Rules == nil {
		c.Rules = Default().Rules
	}
}

// EnsureDirectories creates the download root and every category directory.
// Missing directories are a normal condition (the user may have deleted one),
// so this is safe to call on every start.
func EnsureDirectories(cfg Config) error {
	if err := os.MkdirAll(cfg.DownloadRoot, 0o755); err != nil {
		return fmt.Errorf("create download root: %w", err)
	}
	for _, cat := range cfg.Categories {
		if strings.TrimSpace(cat.Dir) == "" {
			continue
		}
		if err := os.MkdirAll(cat.Dir, 0o755); err != nil {
			return fmt.Errorf("create category %q: %w", cat.Name, err)
		}
	}
	return nil
}
