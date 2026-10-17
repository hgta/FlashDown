//go:build !windows

package engine

import "errors"

// errVolumeUnsupported keeps the preallocation decision platform-neutral: where
// volume metadata is unavailable, "auto" simply does not preallocate. That is the
// conservative branch, so nothing user-visible breaks.
var errVolumeUnsupported = errors.New("volume information is only implemented on Windows")

func volumeInfo(string) (string, uint64, error) {
	return "", 0, errVolumeUnsupported
}
