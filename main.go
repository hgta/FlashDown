package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/hgta/flashdown/app"
	"github.com/hgta/flashdown/internal/config"
	"github.com/hgta/flashdown/internal/engine"
	"github.com/hgta/flashdown/internal/startup"
)

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	// Registering the payload types makes the binding generator emit typed
	// TS signatures for these events (see frontend/bindings/.../eventcreate).
	application.RegisterEvent[app.Snapshot](app.EventTasksSnapshot)
	application.RegisterEvent[app.TaskDetail](app.EventTaskDetail)
	application.RegisterEvent[app.SpeedTick](app.EventSpeedTick)
	application.RegisterEvent[app.FilesDropped](app.EventFilesDropped)
	application.RegisterEvent[app.NewTask](app.EventNewTask)
}

func main() {
	demo := flag.Bool("demo", false,
		"run with fabricated tasks so the interface can be built and judged before the engine exists")
	flag.Parse()

	// The GUI subsystem has no console, so every failure below has to reach the
	// screen through startup.Fail: a log line in a void is what made v0.1.0 look
	// like "double-click does nothing" on Windows 8.1.
	info, startErr := startup.CheckFor(startup.ModeGUI, nil, nil)
	if startErr != nil {
		startup.Fail(startErr, info)
	}

	cfg, warning, err := config.Load()
	if err != nil {
		startup.Fail(fmt.Errorf("读取配置失败：%w", err), info)
	}
	// A missing category directory is normal (the user may have deleted one), so
	// this is a warning rather than a fatal error.
	if err := config.EnsureDirectories(cfg); err != nil {
		log.Printf("warn: %v", err)
	}

	// The real engine is the default now. --demo swaps in fabricated tasks so the
	// interface can still be judged without waiting for real bytes.
	eng := engine.New(cfg)
	if err := eng.Restore(); err != nil {
		log.Printf("warn: restore unfinished downloads: %v", err)
	}

	bridge := engineProvider{m: eng}

	var provider app.Provider = bridge
	if *demo {
		provider = app.NewDemoProvider()
	}

	service := app.NewService(cfg, warning)
	// Retry policy and limits apply to runs that start later; only the jet count
	// is adjustable mid-flight, through Service.SetMaxJets.
	service.OnConfigSaved = eng.ApplyConfig
	// Demo mode deliberately leaves the controller unset: the grid shows fabricated
	// rows, so every control must refuse rather than act on tasks that do not exist.
	if !*demo {
		service.Tasks = bridge
	}

	wailsApp := application.New(application.Options{
		Name:        "FlashDown",
		Description: "FlashGet-inspired download manager",
		Services: []application.Service{
			application.NewService(service),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
	})

	// Main window: the six-region FlashGet-style shell.
	mainWin := wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "FlashDown",
		Width:     1000,
		Height:    660,
		MinWidth:  760,
		MinHeight: 440,
		URL:       "/",
	})

	// Floating window: frameless, always on top, absent from the taskbar.
	// It subscribes to the same event stream as the main window, so its speed
	// readout costs no extra IPC (verified by Spike A).
	floatWin := wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:          "FlashDown",
		Width:          220,
		Height:         92,
		Frameless:      true,
		AlwaysOnTop:    true,
		DisableResize:  true,
		EnableFileDrop: true,
		Hidden:         !cfg.UI.FloatWindow,
		URL:            "/?win=float",
		Windows: application.WindowsWindow{
			HiddenOnTaskbar: true,
		},
	})

	hub := app.NewHub(wailsApp, provider)
	service.Attach(wailsApp, mainWin, floatWin, hub)

	// Start the tickers once the runtime is up, so the very first broadcast is
	// not dropped on the floor while the webview is still loading.
	wailsApp.Event.OnApplicationEvent(events.Common.ApplicationStarted,
		func(*application.ApplicationEvent) { hub.Start() })

	runErr := wailsApp.Run()

	// Stop broadcasting first, then let the engine flush what it has. Quitting must
	// never cost the last second of progress (5.7).
	hub.Stop()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	eng.Shutdown(shutdownCtx)
	cancel()

	if runErr != nil {
		startup.Fail(runErr, info)
	}
}
