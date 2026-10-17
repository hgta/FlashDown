package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

// ErrorKind drives the retry decision: a retryable failure is a bad moment, a
// terminal one is a bad URL (design.md D6/2.11).
type ErrorKind int

const (
	// Retryable means trying again may work: a 5xx, a dropped connection, a
	// server that went quiet for a minute.
	Retryable ErrorKind = iota
	// Terminal means trying again cannot work: a 404, a malformed URL, a target
	// that is not writable.
	Terminal
)

func (k ErrorKind) String() string {
	if k == Terminal {
		return "terminal"
	}
	return "retryable"
}

// StatusError carries an HTTP status that made the request fail, so the UI can
// say "404" instead of "something went wrong".
type StatusError struct {
	Code   int
	Status string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("HTTP %s", e.Status)
}

// ErrStalled is reported when no bytes arrived for the stall window.
var ErrStalled = errors.New("no data received for too long")

// ErrRemoteChanged is returned when the resource no longer matches the journal's
// fingerprint - resuming would splice two different files together (2.9). The UI
// is expected to offer "start over" or "resume anyway"; the engine will not pick
// for the user.
var ErrRemoteChanged = errors.New("remote file changed since the download was paused")

// ErrNoRange is returned when a resume is attempted against a server that no
// longer honours Range requests.
var ErrNoRange = errors.New("server no longer supports range requests")

// Classify decides whether err is worth retrying.
func Classify(err error) ErrorKind {
	if err == nil {
		return Terminal
	}

	// A cancelled context is a pause, not a failure; the caller checks for it
	// before consulting this function, but be safe.
	if errors.Is(err, context.Canceled) {
		return Terminal
	}

	var status *StatusError
	if errors.As(err, &status) {
		switch {
		case status.Code == http.StatusRequestTimeout,
			status.Code == http.StatusTooManyRequests:
			return Retryable
		case status.Code >= 500:
			return Retryable
		default:
			return Terminal
		}
	}

	// A body that ended before the block did, or a server that went quiet, is a
	// bad moment rather than a bad URL.
	if errors.Is(err, ErrStalled) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, io.EOF) {
		return Retryable
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		// A timeout, a reset, or a refused connection: all worth another try.
		return Retryable
	}

	// Transport-level failures (EOF mid-body, connection reset) surface as plain
	// errors, so anything that looks like a transport message is retryable.
	msg := err.Error()
	for _, fragment := range []string{
		"connection reset",
		"connection refused",
		"broken pipe",
		"server closed",
		"unexpected EOF",
		"i/o timeout",
	} {
		if strings.Contains(msg, fragment) {
			return Retryable
		}
	}

	return Terminal
}

// statusOf builds a StatusError from a response, draining a little of the body so
// the message carries the server's own explanation when there is one.
func statusOf(resp *http.Response) error {
	snippet := make([]byte, 200)
	n, _ := io.ReadFull(io.LimitReader(resp.Body, int64(len(snippet))), snippet)
	text := strings.TrimSpace(string(snippet[:n]))

	err := &StatusError{Code: resp.StatusCode, Status: resp.Status}
	if text != "" {
		err.Status = fmt.Sprintf("%s (%s)", resp.Status, oneLine(text))
	}
	return err
}

// oneLine keeps an HTML error page from turning into a log wall.
func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 120 {
		s = s[:120] + "…"
	}
	return s
}
