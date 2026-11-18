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

	"github.com/rossop/dataview.nvim/internal/index"
	"github.com/rossop/dataview.nvim/internal/note"
)

type Result struct {
	Updated   int      `json:"updated"`
	Removed   int      `json:"removed"`
	Unchanged int      `json:"unchanged"`
	Warnings  []string `json:"warnings,omitempty"`
}

type job struct {
	rel  string
	info fs.FileInfo
}

// Vault brings ix in line with the vault on disk. Files whose size and mtime
// match the indexed note are not re-read (§V.7); notes no longer on disk are
// removed.
func Vault(root string, ix *index.Index) (Result, error) {
	var res Result
	var jobs []job
	seen := map[string]bool{}

	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
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
			if p != root && strings.HasPrefix(name, ".") {
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
		rel := relPath(root, p)
		seen[rel] = true
		if old := ix.Get(rel); old != nil && old.Size == info.Size() && old.MTime.Equal(info.ModTime()) {
			res.Unchanged++
			return nil
		}
		jobs = append(jobs, job{rel, info})
		return nil
	})
	if err != nil {
		return res, err
	}

	res.Warnings = append(res.Warnings, parseAll(root, jobs, ix)...)
	res.Updated = len(jobs)

	for _, n := range ix.All() {
		if !seen[n.Path] && ix.Remove(n.Path) {
			res.Removed++
		}
	}
	return res, nil
}

// Paths refreshes only the given vault-relative paths, e.g. after a buffer is
// written. Missing files are removed from the index.
func Paths(root string, rels []string, ix *index.Index) Result {
	var res Result
	var jobs []job
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
		jobs = append(jobs, job{rel, info})
	}
	res.Warnings = parseAll(root, jobs, ix)
	res.Updated = len(jobs)
	return res
}

// parseAll reads and parses jobs on a pool bounded by GOMAXPROCS, so memory
// held at once is at most GOMAXPROCS file bodies (§V.5).
func parseAll(root string, jobs []job, ix *index.Index) []string {
	var (
		mu    sync.Mutex
		warns []string
		wg    sync.WaitGroup
	)
	ch := make(chan job)
	workers := runtime.GOMAXPROCS(0)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range ch {
				b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(j.rel)))
				if err != nil {
					mu.Lock()
					warns = append(warns, err.Error())
					mu.Unlock()
					continue
				}
				n, w := note.Parse(j.rel, b)
				n.Size = j.info.Size()
				n.MTime = j.info.ModTime()
				n.CTime = birthTime(j.info)
				ix.Put(n)
				if len(w) > 0 {
					mu.Lock()
					warns = append(warns, w...)
					mu.Unlock()
				}
			}
		}()
	}
	for _, j := range jobs {
		ch <- j
	}
	close(ch)
	wg.Wait()
	return warns
}

func relPath(root, p string) string {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(rel)
}
