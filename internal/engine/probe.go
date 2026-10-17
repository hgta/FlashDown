package engine

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// ProbeResult is everything the engine needs before it can plan a download.
type ProbeResult struct {
	Size      int64  // total bytes; 0 when the server refuses to say
	Resumable bool   // the server honoured a Range request
	ETag      string // fingerprint half 1
	Modified  string // fingerprint half 2
	Filename  string // best guess at a file name, never empty
}

// Probe asks the server what it has.
//
// It sends `Range: bytes=0-0` rather than a HEAD, because a HEAD tells us
// nothing reliable about range support: some servers answer HEAD with an
// Accept-Ranges header they then ignore on GET. Asking for one byte answers both
// questions at once - 206 means ranges work and `Content-Range` carries the total
// size, 200 means the server ignored the range and we must download in one piece.
func Probe(ctx context.Context, client *http.Client, rawURL string) (ProbeResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return ProbeResult{}, err
	}
	req.Header.Set("Range", "bytes=0-0")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("User-Agent", UserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return ProbeResult{}, err
	}
	defer func() {
		// Drain a little so the connection returns to the pool instead of being
		// torn down; never read a whole body here.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		_ = resp.Body.Close()
	}()

	out := ProbeResult{
		ETag:     resp.Header.Get("ETag"),
		Modified: resp.Header.Get("Last-Modified"),
		Filename: filenameFrom(resp),
	}

	switch resp.StatusCode {
	case http.StatusPartialContent:
		out.Resumable = true
		if total, ok := contentRangeTotal(resp.Header.Get("Content-Range")); ok {
			out.Size = total
		} else {
			out.Size = -1 // ranges work but the total is unknown
		}

	case http.StatusOK:
		// The server ignored the range: this download has to run from byte 0 in
		// one connection, and a resume would have to start over.
		out.Resumable = false
		out.Size = resp.ContentLength

	case http.StatusRequestedRangeNotSatisfiable:
		// An empty file answers a range request this way: `Content-Range: bytes */0`.
		if total, ok := contentRangeTotal(resp.Header.Get("Content-Range")); ok {
			out.Size = total
			out.Resumable = true
		}

	default:
		return ProbeResult{}, statusOf(resp)
	}

	if out.Size == 0 && resp.ContentLength > 0 {
		out.Size = resp.ContentLength
	}
	if out.Size < 0 && resp.ContentLength > 0 {
		out.Size = resp.ContentLength
	}
	return out, nil
}

// contentRangeTotal parses `bytes 0-0/12345` (and the `bytes * /12345` form) and
// reports the total. `*` means the server will not say.
func contentRangeTotal(header string) (int64, bool) {
	_, spec, ok := strings.Cut(header, " ")
	if !ok {
		spec = header
	}
	_, total, ok := strings.Cut(spec, "/")
	if !ok || total == "*" || total == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.TrimSpace(total), 10, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// filenameFrom prefers what the server says, then the URL, and never returns an
// empty string.
func filenameFrom(resp *http.Response) string {
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		// ParseMediaType decodes RFC 5987's filename*=UTF-8''... into plain text.
		if _, params, err := mime.ParseMediaType(cd); err == nil {
			if name := sanitizeName(params["filename"]); name != "" {
				return name
			}
		}
	}

	if resp.Request != nil && resp.Request.URL != nil {
		if name := nameFromURL(resp.Request.URL); name != "" {
			return name
		}
	}
	return "download"
}

// nameFromURL takes the last path segment. A URL ending in "/" has no name, so
// the host is a better guess than "download".
func nameFromURL(u *url.URL) string {
	base := path.Base(u.Path)
	if base == "" || base == "/" || base == "." {
		return sanitizeName(u.Hostname())
	}
	if decoded, err := url.PathUnescape(base); err == nil {
		base = decoded
	}
	return sanitizeName(base)
}

// invalidNameChars are the ones Windows refuses in a file name, plus the control
// range. `/` and `\` are separators, so letting them through would be a path
// traversal bug rather than a cosmetic one.
func sanitizeName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}

	var b strings.Builder
	for _, r := range name {
		switch {
		case r < 0x20:
			// control characters
		case strings.ContainsRune(`<>:"/\|?*`, r):
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}

	// Trailing dots and spaces are legal to write but invisible in Explorer, and
	// Windows refuses to open such files.
	out := strings.Trim(b.String(), " .")
	if out == "" || out == "." || out == ".." {
		return ""
	}
	if len(out) > 180 {
		out = out[:180]
	}
	return out
}

// uniqueName appends " (2)", " (3)" ... until the name is free, so finishing a
// second copy of the same file never overwrites the first (2.12).
func uniqueName(dir, name string) string {
	ext := path.Ext(name)
	stem := strings.TrimSuffix(name, ext)

	candidate := name
	for i := 2; i < 1000; i++ {
		if _, err := os.Lstat(filepath.Join(dir, candidate)); os.IsNotExist(err) {
			return candidate
		}
		candidate = fmt.Sprintf("%s (%d)%s", stem, i, ext)
	}
	return name
}
