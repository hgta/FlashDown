//go:build !windows

package startup

// messageBox has no portable equivalent; Fail has already written to stderr.
func messageBox(title, body, link string) {}
