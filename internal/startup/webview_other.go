//go:build !windows

package startup

// DetectWebView2 reports a placeholder so Check does not refuse to start on
// platforms that are not shipped today.
func DetectWebView2() WebView2Runtime {
	return WebView2Runtime{Version: "n/a", Source: "unsupported-platform"}
}
