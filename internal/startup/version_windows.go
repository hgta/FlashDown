//go:build windows

package startup

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// rtlOSVersionInfo mirrors RTL_OSVERSIONINFOW.
type rtlOSVersionInfo struct {
	OSVersionInfoSize uint32
	MajorVersion      uint32
	MinorVersion      uint32
	BuildNumber       uint32
	PlatformID        uint32
	CSDVersion        [128]uint16
}

var (
	ntdll             = windows.NewLazySystemDLL("ntdll.dll")
	procRtlGetVersion = ntdll.NewProc("RtlGetVersion")
)

// ReadOSVersion asks ntdll for the real version.
//
// GetVersionEx is not an option: a compatibility manifest makes it lie, which is
// exactly what happens on the machines this check exists for. The registry value
// CurrentMajorVersionNumber is not an option either - it only exists on Windows
// 10, so Windows 8.1 would report nonsense.
func ReadOSVersion() (major, minor, build int, ok bool) {
	info := rtlOSVersionInfo{OSVersionInfoSize: uint32(unsafe.Sizeof(rtlOSVersionInfo{}))}

	// RtlGetVersion returns an NTSTATUS; zero means success.
	status, _, _ := procRtlGetVersion.Call(uintptr(unsafe.Pointer(&info)))
	if status != 0 || info.MajorVersion == 0 {
		return 0, 0, 0, false
	}

	return int(info.MajorVersion), int(info.MinorVersion), int(info.BuildNumber), true
}
