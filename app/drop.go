package app

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// forwardFileDrops bridges Explorer drags into the UI.
//
// A file dragged out of Explorer never reaches the DOM: Wails intercepts the
// native drop and delivers it to Go only, so without this hop the drop basket
// would swallow the gesture in silence (design.md D12). Link drags from a
// browser are a different path entirely and are handled in the WebView.
//
// Both windows are hooked because the spec requires dropping onto the main
// window to behave identically to the floating one.
func (s *Service) forwardFileDrops() {
	if s.app == nil {
		return
	}

	handle := func(event *application.WindowEvent) {
		if event.Context() == nil {
			return
		}
		files := event.Context().DroppedFiles()
		if len(files) == 0 {
			return
		}
		s.app.Event.Emit(EventFilesDropped, FilesDropped{Files: files})
	}

	// The returned unsubscribe functions are intentionally dropped: these live
	// for the lifetime of the process.
	if s.main != nil {
		s.main.OnWindowEvent(events.Common.WindowFilesDropped, handle)
	}
	if s.float != nil {
		s.float.OnWindowEvent(events.Common.WindowFilesDropped, handle)
	}
}
