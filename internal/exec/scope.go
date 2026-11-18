package exec

import (
	"regexp"
	"time"

	"github.com/rossop/dataview.nvim/internal/note"
)

// scope resolves bare identifiers for one row.
type scope interface {
	lookup(name string) (any, bool)
	path() string
	page() *pageScope
}

type pageScope struct {
	e    *executor
	n    *note.Note
	file map[string]any // built on first use of file.*
}

func (e *executor) page(n *note.Note) *pageScope {
	if p, ok := e.pages[n.Path]; ok {
		return p
	}
	p := &pageScope{e: e, n: n}
	e.pages[n.Path] = p
	return p
}

func (p *pageScope) path() string     { return p.n.Path }
func (p *pageScope) page() *pageScope { return p }

// lookup order: file, frontmatter, inline fields.
func (p *pageScope) lookup(name string) (any, bool) {
	if name == "file" {
		return p.fileObject(), true
	}
	if v, ok := p.n.Frontmatter.Get(name); ok {
		return p.e.resolve(v, p.n.Path), true
	}
	if v, ok := p.n.Fields.Get(name); ok {
		return p.e.resolve(v, p.n.Path), true
	}
	return nil, false
}

var dayInName = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)

func (p *pageScope) fileObject() map[string]any {
	if p.file != nil {
		return p.file
	}
	e, n := p.e, p.n
	mtime, ctime := e.wall(n.MTime), e.wall(n.CTime)

	var outlinks []any
	seen := map[string]bool{}
	for _, l := range n.Links {
		r := e.resolveLink(l, n.Path)
		if !seen[r.Path] {
			seen[r.Path] = true
			outlinks = append(outlinks, r)
		}
	}
	var inlinks []any
	for _, src := range e.ix.Inlinks(n.Path) {
		inlinks = append(inlinks, note.Link{Path: src})
	}
	tasks := make([]any, len(n.Tasks))
	for i := range n.Tasks {
		tasks[i] = e.taskObject(n, &n.Tasks[i])
	}
	var day any
	if m := dayInName.FindString(n.Name()); m != "" {
		if t, err := time.Parse("2006-01-02", m); err == nil {
			day = t
		}
	}

	p.file = map[string]any{
		"name":        n.Name(),
		"path":        n.Path,
		"folder":      n.Folder(),
		"ext":         "md",
		"link":        note.Link{Path: n.Path},
		"size":        float64(n.Size),
		"mtime":       mtime,
		"mday":        truncDay(mtime),
		"ctime":       ctime,
		"cday":        truncDay(ctime),
		"day":         day,
		"tags":        strList(expandTags(n.Tags)),
		"etags":       strList(n.Tags),
		"aliases":     strList(n.Aliases),
		"outlinks":    outlinks,
		"inlinks":     inlinks,
		"tasks":       tasks,
		"frontmatter": fieldsMap(e, n.Frontmatter, n.Path),
	}
	return p.file
}

type taskScope struct {
	pg  *pageScope
	t   *note.Task
	obj map[string]any
}

func (t *taskScope) path() string     { return t.pg.n.Path }
func (t *taskScope) page() *pageScope { return t.pg }

// Task rows see the task's own fields first, then the page's.
func (t *taskScope) lookup(name string) (any, bool) {
	if t.obj == nil {
		t.obj = t.pg.e.taskObject(t.pg.n, t.t)
	}
	if v, ok := t.obj[name]; ok {
		return v, true
	}
	return t.pg.lookup(name)
}

func (e *executor) taskObject(n *note.Note, t *note.Task) map[string]any {
	links := make([]any, len(t.Links))
	for i, l := range t.Links {
		links[i] = e.resolveLink(l, n.Path)
	}
	obj := map[string]any{
		"text":      t.Text,
		"status":    t.Status,
		"completed": t.Completed,
		"checked":   t.Status != " ",
		"line":      float64(t.Line),
		"path":      n.Path,
		"link":      note.Link{Path: n.Path},
		"tags":      strList(t.Tags),
		"outlinks":  links,
		"task":      true,
	}
	for _, f := range t.Fields {
		obj[f.Key] = e.resolve(f.Value, n.Path)
		if c := note.Canonical(f.Key); c != f.Key {
			obj[c] = obj[f.Key]
		}
	}
	return obj
}

// pageObject is what [[Link]].field sees: frontmatter, inline fields and file.
func (e *executor) pageObject(n *note.Note) map[string]any {
	obj := map[string]any{"file": e.page(n).fileObject()}
	for _, fs := range []note.Fields{n.Fields, n.Frontmatter} { // frontmatter wins
		for _, f := range fs {
			obj[f.Key] = e.resolve(f.Value, n.Path)
		}
	}
	return obj
}

func fieldsMap(e *executor, fs note.Fields, from string) map[string]any {
	m := make(map[string]any, len(fs))
	for _, f := range fs {
		m[f.Key] = e.resolve(f.Value, from)
	}
	return m
}

// resolve turns stored links into vault paths so they compare and navigate
// correctly; other values pass through.
func (e *executor) resolve(v any, from string) any {
	switch x := v.(type) {
	case note.Link:
		return e.resolveLink(x, from)
	case []any:
		out := make([]any, len(x))
		for i, el := range x {
			out[i] = e.resolve(el, from)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, el := range x {
			out[k] = e.resolve(el, from)
		}
		return out
	}
	return v
}

func (e *executor) resolveLink(l note.Link, from string) note.Link {
	if p := e.ix.Resolve(l, from); p != "" {
		l.Path = p
	}
	return l
}

// wall converts an instant to a zone-less wall-clock time so it compares
// with dates written in notes, which carry no zone.
func (e *executor) wall(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	t = t.In(e.opt.Loc)
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.UTC)
}

func truncDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func strList(s []string) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}
