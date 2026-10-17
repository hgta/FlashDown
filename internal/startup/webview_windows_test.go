//go:build windows

package startup

import "testing"

func TestDetectWebView2PrefersRegistry(t *testing.T) {
	got := detectWebView2(
		func(path string) string {
			if path == webView2RegistryPaths[0].path {
				return "120.0.2210.91"
			}
			return ""
		},
		func() string { return "99.0.0.0" },
	)

	if got.Version != "120.0.2210.91" || got.Source != "registry" {
		t.Errorf("got %+v, want the 64-bit registry entry", got)
	}
}

func TestDetectWebView2UsesWow64View(t *testing.T) {
	got := detectWebView2(
		func(path string) string {
			if path == webView2RegistryPaths[1].path {
				return "118.0.2088.76"
			}
			return ""
		},
		func() string { return "99.0.0.0" },
	)

	if got.Version != "118.0.2088.76" || got.Source != "registry-32" {
		t.Errorf("got %+v, want the WOW6432Node entry", got)
	}
}

func TestDetectWebView2FallsBackToDisk(t *testing.T) {
	got := detectWebView2(
		func(string) string { return "" },
		func() string { return "109.0.1518.78" },
	)

	if got.Version != "109.0.1518.78" || got.Source != "disk" {
		t.Errorf("got %+v, want the disk directory", got)
	}
}

func TestDetectWebView2ReportsNothing(t *testing.T) {
	got := detectWebView2(
		func(string) string { return "" },
		func() string { return "" },
	)

	if got.Version != "" {
		t.Errorf("got %+v, want an empty result", got)
	}
}
