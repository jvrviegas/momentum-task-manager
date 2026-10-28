package calendar

import (
	"errors"
	"io/fs"
	"time"
)

// FileState is what loading one calendar file on its own found.
type FileState string

const (
	// FileAvailable: the file reads and parses, and changed recently enough.
	FileAvailable FileState = "available"
	// FileStale: it reads and parses, but last changed longer ago than
	// staleAfter, so its meetings may be out of date.
	FileStale FileState = "stale"
	// FileMissing: there is no such file.
	FileMissing FileState = "missing"
	// FileUnreadable: it exists but cannot be read or parsed.
	FileUnreadable FileState = "error"
)

// FileStatus describes one file. Detail says why an unreadable file is, and
// Modified when an existing file last changed.
type FileStatus struct {
	State    FileState
	Detail   string
	Modified time.Time
}

// Inspect loads one file the way Load does and says what it found. It never
// reports an event or a title.
func Inspect(path string, now time.Time, staleAfter time.Duration) FileStatus {
	_, modified, stale, err := readFile(path, now, staleAfter)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return FileStatus{State: FileMissing}
	case err != nil:
		return FileStatus{State: FileUnreadable, Detail: reason(err), Modified: modified}
	case stale:
		return FileStatus{State: FileStale, Modified: modified}
	default:
		return FileStatus{State: FileAvailable, Modified: modified}
	}
}

// reason is the part of a read or parse error that is not the path, which
// the app already shows.
func reason(err error) string {
	inner := errors.Unwrap(err)
	if inner == nil {
		return err.Error()
	}
	var pathErr *fs.PathError
	if errors.As(inner, &pathErr) {
		return pathErr.Err.Error()
	}
	return inner.Error()
}
