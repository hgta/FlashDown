package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	// DataSuffix marks a file whose download is not finished - FlashGet's `.jc!`
	// idea, with the real extension kept visible (design.md D9).
	DataSuffix = ".fd!"
	// JournalSuffix is appended to the data path, giving `<name>.fd!.state`.
	JournalSuffix = ".state"

	journalVersion = 1
)

// ErrJournalCorrupt means the state file was unreadable or from another version.
// The caller decides whether to start over or ask the user.
var ErrJournalCorrupt = errors.New("download state file is unreadable")

// Paths returns the in-progress data file and its journal for a task name.
func Paths(dir, name string) (dataPath, journalPath string) {
	dataPath = filepath.Join(dir, name+DataSuffix)
	return dataPath, dataPath + JournalSuffix
}

// Journal is the crash-recovery record. It is the only thing that knows how much
// of a half-downloaded file is trustworthy, so it is written atomically and
// flushed on a timer (design.md D9).
type Journal struct {
	Version   int     `json:"version"`
	URL       string  `json:"url"`
	Name      string  `json:"name"`
	Size      int64   `json:"size"`
	ETag      string  `json:"etag"`
	Modified  string  `json:"lastModified"`
	Resumable bool    `json:"resumable"`
	Blocks    []Block `json:"blocks"`
	UpdatedAt int64   `json:"updatedAtMillis"`
}

// Done is how many bytes the journal claims are safely on disk.
func (j *Journal) Done() int64 { return TotalDone(j.Blocks) }

// LoadJournal reads a state file.
//
// A missing file returns the os error unchanged so the caller can test for
// os.IsNotExist; a damaged one is moved aside and reported as ErrJournalCorrupt.
// Keeping the .bak matters: the journal is the only record of progress, so it is
// worth having around when something needs diagnosing.
func LoadJournal(path string) (*Journal, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var j Journal
	if err := json.Unmarshal(raw, &j); err != nil {
		_ = os.Rename(path, path+".bak")
		return nil, fmt.Errorf("%w: %v", ErrJournalCorrupt, err)
	}
	if j.Version != journalVersion {
		_ = os.Rename(path, path+".bak")
		return nil, fmt.Errorf("%w: version %d", ErrJournalCorrupt, j.Version)
	}

	// Jet is not serialised, so every block comes back with the zero value, which
	// reads as "claimed by jet 0". Restore them to idle.
	ClearJets(j.Blocks)
	return &j, nil
}

// Save writes the journal atomically: temp file, fsync, rename. A crash at any
// point leaves either the previous journal or the new one, never a half of each.
func (j *Journal) Save(path string) error {
	j.Version = journalVersion
	j.UpdatedAt = time.Now().UnixMilli()

	buf, err := json.Marshal(j)
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}

	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("create %s: %w", tmp, err)
	}
	if _, err := f.Write(buf); err != nil {
		_ = f.Close()
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

// Remove deletes the journal, ignoring a file that is already gone.
func RemoveJournal(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
