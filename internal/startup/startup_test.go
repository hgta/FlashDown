package startup

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func fixedVersion(major, minor, build int, ok bool) VersionReader {
	return func() (int, int, int, bool) { return major, minor, build, ok }
}

func fixedWebView(version string) WebView2Reader {
	return func() WebView2Runtime {
		return WebView2Runtime{Version: version, Source: "test"}
	}
}

func TestCheckAcceptsSupportedWindows(t *testing.T) {
	info, err := Check(fixedVersion(10, 0, 19045, true), fixedWebView("120.0.2210.91"))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if info.OSVersion != "10.0.19045" {
		t.Errorf("OSVersion = %q, want 10.0.19045", info.OSVersion)
	}
	if info.WebView2 != "120.0.2210.91" {
		t.Errorf("WebView2 = %q", info.WebView2)
	}
}

func TestCheckAcceptsWindows11(t *testing.T) {
	if _, err := Check(fixedVersion(10, 0, 22631, true), fixedWebView("120.0")); err != nil {
		t.Fatalf("Windows 11 should pass: %v", err)
	}
}

func TestCheckRefusesWindows81(t *testing.T) {
	info, err := Check(fixedVersion(6, 3, 9600, true), fixedWebView("109.0.1518.78"))

	var unsupported *UnsupportedError
	if !errors.As(err, &unsupported) {
		t.Fatalf("err = %v, want UnsupportedError", err)
	}
	if unsupported.Version != "6.3.9600" {
		t.Errorf("version = %q, want 6.3.9600", unsupported.Version)
	}
	// The dialog must still be able to name the system it refused.
	if info.VersionText() != "6.3.9600" {
		t.Errorf("info.VersionText() = %q", info.VersionText())
	}
}

func TestCheckRefusesWindows10Before1809(t *testing.T) {
	if _, err := Check(fixedVersion(10, 0, 17134, true), fixedWebView("120.0")); err == nil {
		t.Fatal("build 17134 is older than 17763 and must be refused")
	}
}

func TestCheckAllowsUnknownVersion(t *testing.T) {
	if _, err := Check(fixedVersion(0, 0, 0, false), fixedWebView("120.0")); err != nil {
		t.Fatalf("unknown version must not block startup: %v", err)
	}
}

func TestCheckRefusesMissingWebView2(t *testing.T) {
	_, err := Check(fixedVersion(10, 0, 19045, true), fixedWebView(""))
	if !errors.Is(err, ErrWebView2Missing) {
		t.Fatalf("err = %v, want ErrWebView2Missing", err)
	}
}

func TestDescribeUnsupportedNamesVersionAndLog(t *testing.T) {
	title, body, link := Describe(
		&UnsupportedError{Version: "6.3.9600"},
		Info{OSVersion: "6.3.9600", OSVersionKnown: true},
	)

	if title != "FlashDown 无法启动" {
		t.Errorf("title = %q", title)
	}
	for _, want := range []string{"6.3.9600", "Windows 10 1809", "startup.log"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
	if link != "" {
		t.Errorf("an unsupported OS has nothing to install, link = %q", link)
	}
}

func TestDescribeOffersWebView2Download(t *testing.T) {
	_, body, link := Describe(ErrWebView2Missing, Info{OSVersion: "10.0.19045", OSVersionKnown: true})

	if link != WebView2DownloadURL {
		t.Errorf("link = %q, want the WebView2 download page", link)
	}
	if !strings.Contains(body, "WebView2") {
		t.Errorf("body does not mention WebView2:\n%s", body)
	}
}

func TestDescribeFallsBackToRawError(t *testing.T) {
	_, body, _ := Describe(errors.New("failed to create window"), Info{})

	if !strings.Contains(body, "failed to create window") {
		t.Errorf("raw error missing from body:\n%s", body)
	}
	if !strings.Contains(body, "未知") {
		t.Errorf("unknown OS version should be stated explicitly:\n%s", body)
	}
}

func TestWriteLogToAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "startup.log")

	if err := writeLogTo(path, "first"); err != nil {
		t.Fatalf("writeLogTo: %v", err)
	}
	if err := writeLogTo(path, "second"); err != nil {
		t.Fatalf("writeLogTo: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 || lines[0] != "first" || lines[1] != "second" {
		t.Errorf("log = %q", string(data))
	}
}

func TestWriteLogToTruncatesOversizedLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "startup.log")
	big := strings.Repeat("x", logMaxBytes+16)
	if err := os.WriteFile(path, []byte(big), 0o644); err != nil {
		t.Fatalf("seed log: %v", err)
	}

	if err := writeLogTo(path, "after-truncate"); err != nil {
		t.Fatalf("writeLogTo: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "after-truncate" {
		t.Errorf("log = %q, want only the new line", got)
	}
}

// The fakes above prove the decision logic; this proves the plumbing - the
// RtlGetVersion call convention and the registry paths - against a real machine.
func TestRealProbesOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}

	major, minor, build, ok := ReadOSVersion()
	if !ok {
		t.Fatal("RtlGetVersion failed on Windows")
	}
	if major < 10 {
		t.Fatalf("version = %d.%d.%d, expected Windows 10 or newer", major, minor, build)
	}

	runtimeInfo := DetectWebView2()
	if runtimeInfo.Version == "" {
		t.Skip("this machine has no WebView2, so only the version probe is verified")
	}
	if runtimeInfo.Source == "" {
		t.Errorf("WebView2 %s has no source recorded", runtimeInfo.Version)
	}
}

func TestStageOfLabelsTheCheck(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{&UnsupportedError{Version: "6.3.9600"}, "version-check"},
		{ErrWebView2Missing, "webview2-check"},
		{errors.New("boom"), "run"},
	}
	for _, c := range cases {
		if got := stageOf(c.err); got != c.want {
			t.Errorf("stageOf(%v) = %q, want %q", c.err, got, c.want)
		}
	}
}
