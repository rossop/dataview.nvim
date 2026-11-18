package scan

import (
	"io/fs"
	"syscall"
	"time"
)

func birthTime(fi fs.FileInfo) time.Time {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return time.Unix(st.Birthtimespec.Unix())
	}
	return fi.ModTime()
}
