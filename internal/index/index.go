// Package index holds every parsed note in memory and answers vault-wide
// questions (link resolution, inlinks). Pure core: callers feed it notes;
// it never touches the file system (§V.10).
package index

import (
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/rossop/dataview.nvim/internal/note"
)

type Index struct {
	mu    sync.RWMutex
	notes map[string]*note.Note
	// byName maps a lowercased file name (no .md) to its paths, so link
	// resolution is O(candidates) rather than O(vault).
	byName map[string][]string
	// inlinks is derived from notes and rebuilt lazily after any change, so
	// writes stay O(1) and only the first inlink lookup after a change pays.
	inlinks map[string][]string
}

func New() *Index {
	return &Index{notes: map[string]*note.Note{}, byName: map[string][]string{}}
}

func nameKey(p string) string {
	return strings.ToLower(strings.TrimSuffix(path.Base(p), ".md"))
}

func (ix *Index) Put(n *note.Note) {
	ix.mu.Lock()
	if _, exists := ix.notes[n.Path]; !exists {
		k := nameKey(n.Path)
		ix.byName[k] = append(ix.byName[k], n.Path)
	}
	ix.notes[n.Path] = n
	ix.inlinks = nil
	ix.mu.Unlock()
}

func (ix *Index) Remove(p string) bool {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	if _, ok := ix.notes[p]; !ok {
		return false
	}
	delete(ix.notes, p)
	k := nameKey(p)
	paths := ix.byName[k]
	for i, q := range paths {
		if q == p {
			paths = append(paths[:i], paths[i+1:]...)
			break
		}
	}
	if len(paths) == 0 {
		delete(ix.byName, k)
	} else {
		ix.byName[k] = paths
	}
	ix.inlinks = nil
	return true
}

func (ix *Index) Get(p string) *note.Note {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return ix.notes[p]
}

func (ix *Index) Len() int {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return len(ix.notes)
}

// All returns every note sorted by path, so query output is deterministic.
func (ix *Index) All() []*note.Note {
	ix.mu.RLock()
	out := make([]*note.Note, 0, len(ix.notes))
	for _, n := range ix.notes {
		out = append(out, n)
	}
	ix.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// Resolve maps a link written in note `from` to a vault path ("x/y.md").
// It follows Obsidian: exact path first, then the shortest path whose tail
// matches, preferring the source note's folder. Unresolvable links return "".
func (ix *Index) Resolve(l note.Link, from string) string {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return ix.resolve(l.Path, from)
}

func (ix *Index) resolve(target, from string) string {
	if target == "" {
		return from
	}
	target = strings.TrimSuffix(target, ".md")
	if n, ok := ix.notes[target+".md"]; ok {
		return n.Path
	}
	want := strings.ToLower(target)
	fromDir := path.Dir(from)
	best := ""
	bestScore := 0
	for _, p := range ix.byName[nameKey(want)] {
		lp := strings.ToLower(strings.TrimSuffix(p, ".md"))
		if lp != want && !strings.HasSuffix(lp, "/"+want) {
			continue
		}
		// Higher is better: same folder beats everything, then shorter paths;
		// path order breaks ties so the result is deterministic.
		score := 1 << 20
		if path.Dir(p) == fromDir {
			score += 1 << 19
		}
		score -= len(p)
		if score > bestScore || (score == bestScore && p < best) {
			best, bestScore = p, score
		}
	}
	return best
}

// Inlinks lists notes that link to p, sorted.
func (ix *Index) Inlinks(p string) []string {
	ix.mu.RLock()
	if ix.inlinks != nil {
		defer ix.mu.RUnlock()
		return ix.inlinks[p]
	}
	ix.mu.RUnlock()

	ix.mu.Lock()
	defer ix.mu.Unlock()
	if ix.inlinks == nil {
		ix.buildInlinks()
	}
	return ix.inlinks[p]
}

func (ix *Index) buildInlinks() {
	in := map[string][]string{}
	for src, n := range ix.notes {
		seen := map[string]bool{}
		for _, l := range n.Links {
			dst := ix.resolve(l.Path, src)
			if dst == "" || dst == src || seen[dst] {
				continue
			}
			seen[dst] = true
			in[dst] = append(in[dst], src)
		}
	}
	for _, v := range in {
		sort.Strings(v)
	}
	ix.inlinks = in
}
