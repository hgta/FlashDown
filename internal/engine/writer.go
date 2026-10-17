package engine

import (
	"fmt"
	"os"
	"path/filepath"
)

// Preallocation policies, mirroring internal/config's strings.
const (
	PreallocateAuto   = "auto"
	PreallocateAlways = "always"
	PreallocateNever  = "never"
)

// Writer owns the in-progress data file.
//
// Every write is positional, which is what makes N connections writing the same
// file at once safe (verified by Spike C: 30 writers, zero misplaced blocks).
type Writer struct {
	f            *os.File
	path         string
	size         int64
	preallocated bool
}

// OpenWriter opens, or creates, the data file for size bytes.
//
// It never truncates on open: on a resume the bytes already on disk are the whole
// point. `policy` follows config's auto/always/never.
func OpenWriter(path string, size int64, policy string) (*Writer, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create %s: %w", dir, err)
		}
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}

	w := &Writer{f: f, path: path, size: size}

	// If the resource shrank since the last run, cut the stray tail now: the
	// finished file must be exactly the size the server reports.
	if size > 0 {
		if info, statErr := f.Stat(); statErr == nil && info.Size() > size {
			if err := w.Truncate(size); err != nil {
				_ = f.Close()
				return nil, err
			}
		}
	}

	if w.shouldPreallocate(policy) {
		if err := w.Truncate(size); err != nil {
			_ = f.Close()
			return nil, err
		}
		w.preallocated = true
	}

	return w, nil
}

// shouldPreallocate implements design.md D4.
//
// The spike result that shaped this: on NTFS `Truncate` is sub-millisecond but
// immediately claims the full file size, so the cost of preallocating is disk
// space, not time. "auto" therefore decides from free bytes, and only on NTFS,
// because that is the only filesystem the assumption was measured on.
func (w *Writer) shouldPreallocate(policy string) bool {
	if w.size <= 0 {
		return false
	}

	switch policy {
	case PreallocateAlways:
		return true
	case PreallocateNever:
		return false
	}

	fs, free, err := volumeInfo(w.path)
	if err != nil || fs != "NTFS" {
		return false
	}
	// The 1.2 headroom keeps a download from filling the volume to the last byte.
	return float64(free) >= float64(w.size)*1.2
}

// WriteAt writes p at the absolute file offset off.
func (w *Writer) WriteAt(p []byte, off int64) (int, error) {
	if w.f == nil {
		return 0, os.ErrClosed
	}
	return w.f.WriteAt(p, off)
}

// Truncate resizes the file. Used for preallocation and when the remote shrank.
func (w *Writer) Truncate(size int64) error {
	if w.f == nil {
		return os.ErrClosed
	}
	if err := w.f.Truncate(size); err != nil {
		return fmt.Errorf("truncate %s: %w", w.path, err)
	}
	w.size = size
	return nil
}

// Sync flushes the file before the journal claims the bytes are on disk.
func (w *Writer) Sync() error {
	if w.f == nil {
		return nil
	}
	return w.f.Sync()
}

// Close is idempotent: the runner defers it and finish() also closes, and a
// double close must not turn a finished download into a failure.
func (w *Writer) Close() error {
	if w.f == nil {
		return nil
	}
	err := w.f.Close()
	w.f = nil
	return err
}

// Path is the on-disk data file.
func (w *Writer) Path() string { return w.path }

// Preallocated reports whether the full size was reserved up front, which the
// log panel shows because it explains the disk usage spike.
func (w *Writer) Preallocated() bool { return w.preallocated }

// Size is the length the writer currently believes the file should have.
func (w *Writer) Size() int64 { return w.size }
