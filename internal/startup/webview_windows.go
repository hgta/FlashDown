//go:build windows

package startup

import (
	"os"
	"path/filepath"
	"regexp"

	"golang.org/x/sys/windows/registry"
)

// webview2ClientID is the fixed GUID the Evergreen installer registers under.
const webview2ClientID = `{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`

var versionDirPattern = regexp.MustCompile(`^\d+\.\d+\.\d+`)

// DetectWebView2 looks in the two registry views, then on disk.
//
// The disk fallback matters for enterprise images that copy the runtime in
// without running the installer: judging by registry alone would refuse a
// machine that works perfectly well.
func DetectWebView2() WebView2Runtime {
	return detectWebView2(registryPV, diskWebView2)
}

// detectWebView2 is the fallback chain with its two sources injected, so the
// order can be tested without touching a real registry.
func detectWebView2(readRegistry func(string) string, readDisk func() string) WebView2Runtime {
	for _, candidate := range webView2RegistryPaths {
		if v := readRegistry(candidate.path); v != "" {
			return WebView2Runtime{Version: v, Source: candidate.source}
		}
	}

	if v := readDisk(); v != "" {
		return WebView2Runtime{Version: v, Source: "disk"}
	}

	return WebView2Runtime{}
}

var webView2RegistryPaths = []struct{ path, source string }{
	{`SOFTWARE\Microsoft\EdgeUpdate\Clients\` + webview2ClientID, "registry"},
	{`SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\` + webview2ClientID, "registry-32"},
}

func registryPV(path string) string {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer key.Close()

	version, _, err := key.GetStringValue("pv")
	if err != nil || version == "" {
		return ""
	}
	return version
}

func diskWebView2() string {
	base := os.Getenv("ProgramFiles(x86)")
	if base == "" {
		base = os.Getenv("ProgramFiles")
	}
	if base == "" {
		return ""
	}

	entries, err := os.ReadDir(filepath.Join(base, "Microsoft", "EdgeWebView", "Application"))
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if entry.IsDir() && versionDirPattern.MatchString(entry.Name()) {
			return entry.Name()
		}
	}
	return ""
}
