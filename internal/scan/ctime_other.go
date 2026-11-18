//go:build !darwin

package scan

import (
	"io/fs"
	"time"
)

// birthTime falls back to mtime where the OS exposes no creation time
// through the standard library.
func birthTime(fi fs.FileInfo) time.Time {
	return fi.ModTime()
}
