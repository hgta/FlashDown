package engine

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

// UserAgent identifies the client. Some mirrors behave differently for unknown
// agents, and being honest about who we are costs nothing.
const UserAgent = "FlashDown/0.1 (+https://github.com/hgta/flashdown)"

// NewClient builds the engine's HTTP client.
//
// Three settings are load-bearing (design.md D5) and none of them are defaults:
//
//   - DisableCompression: Go otherwise sends `Accept-Encoding: gzip` and
//     transparently decompresses the body, which makes Content-Length describe
//     the compressed stream and turns byte offsets into nonsense.
//   - HTTP/1.1 pinning: HTTP/2 multiplexes every request over one TCP
//     connection, so "N jets = N connections = N times the bandwidth" is false
//     and the speed becomes unpredictable.
//   - No client-wide timeout: a 20 GB download is not a timeout. Per-request
//     deadlines and the stall detector (see runner.go) cover the cases that
//     actually need a clock.
func NewClient() *http.Client {
	tr := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConnsPerHost:   16,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		DisableCompression:    true,
		ForceAttemptHTTP2:     false,
		// A non-nil empty map is the documented way to tell crypto/tls there are
		// no HTTP/2 upgrades to negotiate, which pins the transport to HTTP/1.1.
		TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{},
	}
	return &http.Client{Transport: tr}
}
