package app

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// FloatMenuID is the id the floating window asks for with the CSS property
// `--custom-contextmenu: float-menu`. The WebView reads that value and looks it
// up in the application's context-menu registry, so this constant and the CSS
// rule in FloatWindow.vue must agree (design.md D11).
const FloatMenuID = "float-menu"

// Wording for greyed-out rows. A greyed row tells the user the command exists,
// which a missing row does not.
const (
	pendingEngine  = "下载引擎接入后可用（M1）"
	pendingOptions = "选项对话框随后提供（M3–M4）"
)

// registerFloatMenu builds the floating window's native right-click menu.
//
// It must run after application.New, since ContextMenu.Update registers itself
// into the process-global application. Building the items here rather than
// calling Update afterwards is deliberate: the Windows and macOS backends
// materialise the popup from this Go-side menu when it is opened, so items added
// after registration are not lost.
func (s *Service) registerFloatMenu() {
	menu := application.NewContextMenu(FloatMenuID)

	menu.Add("显示 / 隐藏主窗口").OnClick(func(*application.Context) { s.toggleMainWindow() })
	menu.Add("新建任务…").SetEnabled(false).SetTooltip(pendingEngine)
	menu.AddSeparator()
	menu.Add("全部开始").SetEnabled(false).SetTooltip(pendingEngine)
	menu.Add("全部暂停").SetEnabled(false).SetTooltip(pendingEngine)
	menu.AddSeparator()
	menu.Add("悬浮窗设置…").SetEnabled(false).SetTooltip(pendingOptions)
	menu.AddSeparator()
	menu.Add("退出").OnClick(func(*application.Context) { s.quitApp() })
}

// toggleMainWindow is what makes the floating window worth having: with the main
// window hidden, this is the only way back to it.
func (s *Service) toggleMainWindow() {
	if s.main == nil {
		return
	}
	if s.main.IsVisible() {
		s.main.Hide()
		return
	}
	s.main.Show()
	s.main.Focus()
}

func (s *Service) quitApp() {
	if s.app == nil {
		return
	}
	s.app.Quit()
}
