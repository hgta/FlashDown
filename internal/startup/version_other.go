//go:build !windows

package startup

// ReadOSVersion cannot answer off Windows; the caller treats that as "unknown"
// and lets the app continue.
func ReadOSVersion() (major, minor, build int, ok bool) {
	return 0, 0, 0, false
}
