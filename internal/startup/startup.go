// Package startup decides whether this machine can run FlashDown at all, and
// makes sure a refusal is something the user can see.
//
// The motivation is concrete: v0.1.0 on Windows 8.1 exits silently, because the
// GUI subsystem has no console and main reported every failure through log.Fatal.
// The checks here run before the app is constructed, so they also cover a missing
// WebView2 runtime - the other way this program can die without a trace.
package startup

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// Windows 10 1809 is the oldest release that both Go 1.21+ and WebView2
	// support; anything older is a hard no rather than a degraded yes.
	MinMajor = 10
	MinMinor = 0
	MinBuild = 17763

	logFileName = "startup.log"
	logMaxBytes = 1 << 20

	// WebView2DownloadURL is Microsoft's evergreen bootstrapper landing page.
	WebView2DownloadURL = "https://go.microsoft.com/fwlink/p/?LinkId=2124703"
)

// ErrWebView2Missing means no WebView2 runtime was found through any of the
// known locations. Wails cannot create a window without it.
var ErrWebView2Missing = errors.New("webview2 runtime not found")

// UnsupportedError means the OS itself is below the supported floor.
type UnsupportedError struct {
	Version string // "6.3.9600", or "未知" when the version could not be read
}

func (e *UnsupportedError) Error() string {
	return fmt.Sprintf("Windows %s is below the supported minimum (10.0.%d)", e.Version, MinBuild)
}

// Mode selects how strict the checks are.
//
// The GUI cannot exist without WebView2, so it is refused on anything the
// runtime does not support. The CLI has no such dependency: it runs anyway and
// leaves a warning, because a download that works beats a refusal that explains.
type Mode int

const (
	// ModeGUI is the strict path used by flashdown.exe.
	ModeGUI Mode = iota
	// ModeCLI skips WebView2 and tolerates an unsupported OS.
	ModeCLI
)

// Info is what the checks learned, kept so the failure dialog and the log can
// state facts instead of guesses.
type Info struct {
	OSVersion      string
	OSVersionKnown bool
	WebView2       string
	WebView2Source string
	// Unsupported is set when the OS is below the floor but the caller is the
	// CLI, which may continue with a warning.
	Unsupported bool
}

// VersionText renders the OS version for humans, admitting when it is unknown.
func (i Info) VersionText() string {
	if !i.OSVersionKnown {
		return "未知"
	}
	return i.OSVersion
}

// WebView2Text renders the runtime version, or a dash when none was found.
func (i Info) WebView2Text() string {
	if i.WebView2 == "" {
		return "-"
	}
	return i.WebView2
}

// VersionReader reports the real OS version. ok=false means "could not tell",
// which is deliberately treated as acceptable: refusing a working machine is
// worse than letting one old machine through.
type VersionReader func() (major, minor, build int, ok bool)

// WebView2Reader reports the runtime it found, if any.
type WebView2Reader func() WebView2Runtime

// WebView2Runtime is a located WebView2 installation.
type WebView2Runtime struct {
	Version string // empty when nothing was found
	Source  string // registry, registry-32, disk, or empty
}

// Check is the GUI policy: refuse anything that cannot show a window.
// A nil reader means "use the real one", which keeps main free of build tags
// and lets tests substitute fakes.
func Check(readVersion VersionReader, readWebView WebView2Reader) (Info, error) {
	return CheckFor(ModeGUI, readVersion, readWebView)
}

// CheckFor applies the policy for one of the two entry points.
//
// GUI: both probes are hard requirements. CLI: no window is ever created, so
// WebView2 is irrelevant and an old OS is only recorded - the caller prints a
// warning and keeps downloading.
func CheckFor(mode Mode, readVersion VersionReader, readWebView WebView2Reader) (Info, error) {
	if readVersion == nil {
		readVersion = ReadOSVersion
	}
	if readWebView == nil {
		readWebView = DetectWebView2
	}

	info := Info{}
	if major, minor, build, ok := readVersion(); ok {
		info.OSVersionKnown = true
		info.OSVersion = fmt.Sprintf("%d.%d.%d", major, minor, build)
		if !atLeast(major, minor, build, MinMajor, MinMinor, MinBuild) {
			if mode == ModeCLI {
				info.Unsupported = true
				return info, nil
			}
			return info, &UnsupportedError{Version: info.OSVersion}
		}
	}

	if mode == ModeCLI {
		return info, nil
	}

	runtime := readWebView()
	info.WebView2 = runtime.Version
	info.WebView2Source = runtime.Source
	if runtime.Version == "" {
		return info, ErrWebView2Missing
	}

	return info, nil
}

// WarnPlatform records that the CLI is running on an OS nobody supports. It
// never blocks and never shows a dialog: a command-line user wants the bytes.
func WarnPlatform(info Info) {
	line := fmt.Sprintf("%s stage=platform-warning os=%s webview2=%s error=%q",
		time.Now().Format(time.RFC3339), info.VersionText(), info.WebView2Text(),
		"running on an unsupported Windows version")
	_ = writeLogTo(LogPath(), line)
}

// atLeast compares dotted versions, which is not the same as comparing each
// component against the requirement: 10.0.22000 must pass 10.0.17763.
func atLeast(major, minor, build, reqMajor, reqMinor, reqBuild int) bool {
	if major != reqMajor {
		return major > reqMajor
	}
	if minor != reqMinor {
		return minor > reqMinor
	}
	return build >= reqBuild
}

// Describe builds the dialog contents and, when there is something to install,
// the link the OK button should open.
func Describe(err error, info Info) (title, body, link string) {
	title = "FlashDown 无法启动"

	var b strings.Builder
	b.WriteString("FlashDown 无法启动。\n\n")

	var unsupported *UnsupportedError
	switch {
	case errors.As(err, &unsupported):
		fmt.Fprintf(&b, "原因：当前系统 Windows %s 不受支持。\n", unsupported.Version)
		fmt.Fprintf(&b, "要求：Windows 10 1809（10.0.%d）或更高版本，64 位。\n", MinBuild)
		b.WriteString("\n如果只想下载文件，可以用同目录下的 flashdown-cli.exe：\n")
		b.WriteString("    flashdown-cli.exe https://example.com/file.zip\n")
	case errors.Is(err, ErrWebView2Missing):
		b.WriteString("原因：未找到 Microsoft Edge WebView2 运行时。\n")
		b.WriteString("Windows 10 / 11 通常已内置；缺失时可以从微软官网免费安装。\n")
		link = WebView2DownloadURL
	default:
		fmt.Fprintf(&b, "原因：%v\n", err)
	}

	fmt.Fprintf(&b, "\n当前系统：Windows %s\n", info.VersionText())
	fmt.Fprintf(&b, "WebView2：%s\n", info.WebView2Text())
	fmt.Fprintf(&b, "\n详细信息：%s\n", LogPath())

	if link != "" {
		b.WriteString("\n确定 = 打开下载页    取消 = 退出")
	}

	return title, b.String(), link
}

// Fail is the single exit for startup problems: say it on stderr when there is
// one, write it to the log, put it on screen, then leave with a non-zero code.
func Fail(err error, info Info) {
	log.Printf("startup: %v", err)

	line := fmt.Sprintf("%s stage=%s os=%s webview2=%s error=%q",
		time.Now().Format(time.RFC3339), stageOf(err), info.VersionText(), info.WebView2Text(), err.Error())
	_ = writeLogTo(LogPath(), line)

	title, body, link := Describe(err, info)
	messageBox(title, body, link)

	os.Exit(1)
}

// stageOf labels the log line so a support engineer knows how far the process got.
func stageOf(err error) string {
	var unsupported *UnsupportedError
	switch {
	case errors.As(err, &unsupported):
		return "version-check"
	case errors.Is(err, ErrWebView2Missing):
		return "webview2-check"
	default:
		return "run"
	}
}

// LogPath is where startup failures are appended, next to the config directory
// but kept separate from config.json.
func LogPath() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "FlashDown", logFileName)
}

// writeLogTo appends one line, truncating a log that grew past the cap. Every
// error here is swallowed: a log failure must never hide the dialog.
func writeLogTo(path, line string) error {
	if dir := filepath.Dir(path); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}

	if fi, err := os.Stat(path); err == nil && fi.Size() > logMaxBytes {
		_ = os.Truncate(path, 0)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = fmt.Fprintln(f, line)
	return err
}
