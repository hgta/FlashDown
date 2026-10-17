//go:build windows

package startup

import (
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	mbOK        = 0x00000000
	mbOKCancel  = 0x00000001
	mbIconError = 0x00000010
	idOK        = 1
)

var (
	user32          = windows.NewLazySystemDLL("user32.dll")
	procMessageBoxW = user32.NewProc("MessageBoxW")
)

// messageBox blocks until the user dismisses it, which is the point: a GUI
// binary has no console, so this is the only way a refusal reaches a person.
//
// The OK / Cancel pair is used when there is something to install: OK opens the
// download page, Cancel just leaves. The body explains that split in words.
func messageBox(title, body, link string) {
	flags := uintptr(mbOK | mbIconError)
	if link != "" {
		flags = mbOKCancel | mbIconError
	}

	caption, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	text, err := windows.UTF16PtrFromString(body)
	if err != nil {
		return
	}

	ret, _, _ := procMessageBoxW.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(caption)), flags)
	if int32(ret) != idOK || link == "" {
		return
	}

	// rundll32 ships with every supported Windows; this avoids binding shell32
	// just to hand a URL to the default browser.
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", link).Start()
}
