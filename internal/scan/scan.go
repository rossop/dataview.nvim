// Package scan is the file-system spoke: it walks a vault and feeds changed
// notes into the index. It only reads (§V.2).
package scan

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/rossop/dataview.nvim/internal/index"
	"github.com/rossop/dataview.nvim/internal/note"
)

type Result struct {
	Updated   int      `json:"updated"`
	Removed   int      `json:"removed"`
	Unchanged int      `json:"unchanged"`
	Warnings  []string `json:"warnings,omitempty"`
}

// job is what a worker needs for one file. Holding fs.FileInfo instead
// (with the OS stat struct inside) for every file in a 10k vault cost ~2 MB.
type job struct {
	rel          string
	size         int64
	mtime, ctime time.Time
}

func newJob(rel string, info fs.FileInfo) job {
	return job{rel, info.Size(), info.ModTime(), birthTime(info)}
}

// Vault brings ix in line with the vault on disk. Files whose size and mtime
// match the indexed note are not re-read (§V.7); notes no longer on disk are
// removed. Files stream from the walk straight to the parse workers, so no
// list of the whole vault is ever held in memory (§V.5).
func Vault(root string, ix *index.Index) (Result, error) {
	var res Result
	// Only needed to find deletions; a first scan has nothing to delete.
	var seen map[string]bool
	if ix.Len() > 0 {
		seen = make(map[string]bool, ix.Len())
	}

	p := startPool(root, ix)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			res.Warnings = append(res.Warnings, err.Error())
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			// Dot folders (.obsidian, .git, .trash) are never notes (§V.14).
			if path != root && strings.HasPrefix(name, ".") {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || !strings.EqualFold(filepath.Ext(name), ".md") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			res.Warnings = append(res.Warnings, err.Error())
			return nil
		}
		rel := relPath(root, path)
		if seen != nil {
			seen[rel] = true
		}
		if old := ix.Get(rel); old != nil && old.Size == info.Size() && old.MTime.Equal(info.ModTime()) {
			res.Unchanged++
			return nil
		}
		p.submit(newJob(rel, info))
		res.Updated++
		return nil
	})
	res.Warnings = append(res.Warnings, p.wait()...)
	if err != nil {
		return res, err
	}

	if seen != nil {
		for _, n := range ix.All() {
			if !seen[n.Path] && ix.Remove(n.Path) {
				res.Removed++
			}
		}
	}
	return res, nil
}

// Paths refreshes only the given vault-relative paths, e.g. after a buffer is
// written. Missing files are removed from the index.
func Paths(root string, rels []string, ix *index.Index) Result {
	var res Result
	p := startPool(root, ix)
	for _, rel := range rels {
		rel = filepath.ToSlash(filepath.Clean(rel))
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			if ix.Remove(rel) {
				res.Removed++
			}
			continue
		}
		if old := ix.Get(rel); old != nil && old.Size == info.Size() && old.MTime.Equal(info.ModTime()) {
			res.Unchanged++
			continue
		}
		p.submit(newJob(rel, info))
		res.Updated++
	}
	res.Warnings = p.wait()
	return res
}

// maxWorkers caps parse parallelism. Past 4 workers the scan gets little
// faster (10k notes: 0.49 s → 0.31 s at 12) but each worker's in-flight
// garbage raises peak RSS (§V.5).
const maxWorkers = 4

type pool struct {
	ch    chan job
	wg    sync.WaitGroup
	mu    sync.Mutex
	warns []string
}

func startPool(root string, ix *index.Index) *pool {
	p := &pool{ch: make(chan job)}
	for range min(runtime.GOMAXPROCS(0), maxWorkers) {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			for j := range p.ch {
				b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(j.rel)))
				if err != nil {
					p.warn(err.Error())
					continue
				}
				n, w := note.Parse(j.rel, b)
				n.Size, n.MTime, n.CTime = j.size, j.mtime, j.ctime
				ix.Put(n)
				p.warn(w...)
			}
		}()
	}
	return p
}

func (p *pool) submit(j job) { p.ch <- j }

func (p *pool) warn(ws ...string) {
	if len(ws) == 0 {
		return
	}
	p.mu.Lock()
	p.warns = append(p.warns, ws...)
	p.mu.Unlock()
}

// wait drains the pool and returns the workers' warnings.
func (p *pool) wait() []string {
	close(p.ch)
	p.wg.Wait()
	return p.warns
}

func relPath(root, p string) string {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(rel)
}
