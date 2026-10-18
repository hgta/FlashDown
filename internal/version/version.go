// Package version holds the release string shared by the GUI and the CLI.
//
// The CLI deliberately does not import app: that package pulls in Wails, and the
// whole point of the CLI is to start without it.
package version

// Value is the user-visible version, matching the git tag minus the "v".
const Value = "0.3.0"
