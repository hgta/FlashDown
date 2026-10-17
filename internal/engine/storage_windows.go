//go:build windows

package engine

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// volumeInfo reports the target volume's filesystem name and the bytes it has
// left, which is what the preallocation decision needs (design.md D4).
func volumeInfo(path string) (string, uint64, error) {
	root, err := windows.UTF16PtrFromString(volumeRoot(path))
	if err != nil {
		return "", 0, err
	}

	var freeToCaller, totalBytes, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(root, &freeToCaller, &totalBytes, &totalFree); err != nil {
		return "", 0, err
	}

	name := make([]uint16, 64)
	var serial, maxComponent, flags uint32
	if err := windows.GetVolumeInformation(
		root,
		nil, 0,
		&serial, &maxComponent, &flags,
		&name[0], uint32(len(name)),
	); err != nil {
		// Free space is still useful even when the name is not available.
		return "", freeToCaller, err
	}

	return strings.ToUpper(windows.UTF16ToString(name)), freeToCaller, nil
}

// volumeRoot turns a directory into "D:\". GetDiskFreeSpaceEx accepts a
// directory, but GetVolumeInformation wants a root path.
func volumeRoot(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	if vol := filepath.VolumeName(abs); vol != "" {
		return vol + `\`
	}
	return abs
}
