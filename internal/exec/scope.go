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

var fileFields = []string{
	"name", "path", "folder", "ext", "link", "size", "mtime", "mday", "ctime", "cday",
	"day", "tags", "etags", "aliases", "outlinks", "inlinks", "tasks", "frontmatter",
}

// fileField computes one file.* value on demand. Queries usually touch one
// or two of these per row, and building them all (every task object, every
// resolved link) for 10k notes cost more memory than the index itself (§V.5).
func (p *pageScope) fileField(name string) (any, bool) {
	e, n := p.e, p.n
	switch name {
	case "name":
		return n.Name(), true
	case "path":
		return n.Path, true
	case "folder":
		return n.Folder(), true
	case "ext":
		return "md", true
	case "link":
		return note.Link{Path: n.Path}, true
	case "size":
		return float64(n.Size), true
	case "mtime":
		return e.wall(n.MTime), true
	case "mday":
		return truncDay(e.wall(n.MTime)), true
	case "ctime":
		return e.wall(n.CTime), true
	case "cday":
		return truncDay(e.wall(n.CTime)), true
	case "day":
		if m := dayInName.FindString(n.Name()); m != "" {
			if t, err := time.Parse("2006-01-02", m); err == nil {
				return t, true
			}
		}
		return nil, true
	case "tags":
		return strList(expandTags(n.Tags)), true
	case "etags":
		return strList(n.Tags), true
	case "aliases":
		return strList(n.Aliases), true
	case "outlinks":
		var out []any
		seen := map[string]bool{}
		for _, l := range n.Links {
			r := e.resolveLink(l, n.Path)
			if !seen[r.Path] {
				seen[r.Path] = true
				out = append(out, r)
			}
		}
		return out, true
	case "inlinks":
		var out []any
		for _, src := range e.ix.Inlinks(n.Path) {
			out = append(out, note.Link{Path: src})
		}
		return out, true
	case "tasks":
		out := make([]any, len(n.Tasks))
		for i := range n.Tasks {
			out[i] = e.taskObject(n, &n.Tasks[i])
		}
		return out, true
	case "frontmatter":
		return fieldsMap(e, n.Frontmatter, n.Path), true
	}
	return nil, false
}

// fileObject materialises every file.* field; only used when a query needs
// `file` as a whole value.
func (p *pageScope) fileObject() map[string]any {
	if p.file == nil {
		p.file = make(map[string]any, len(fileFields))
		for _, k := range fileFields {
			p.file[k], _ = p.fileField(k)
		}
	}
	return p.file
}

type taskScope struct {
	pg *pageScope
	t  *note.Task
}

func (t *taskScope) path() string     { return t.pg.n.Path }
func (t *taskScope) page() *pageScope { return t.pg }

// Task rows see the task's own fields first, then the page's.
func (t *taskScope) lookup(name string) (any, bool) {
	if v, ok := t.pg.e.taskField(t.pg.n, t.t, name); ok {
		return v, true
	}
	return t.pg.lookup(name)
}

// taskField answers one task property without allocating a task object.
func (e *executor) taskField(n *note.Note, t *note.Task, name string) (any, bool) {
	switch name {
	case "text":
		return t.Text, true
	case "status":
		return t.Status, true
	case "completed":
		return t.Completed, true
	case "checked":
		return t.Status != " ", true
	case "line":
		return float64(t.Line), true
	case "path":
		return n.Path, true
	case "link":
		return note.Link{Path: n.Path}, true
	case "tags":
		return strList(t.Tags), true
	case "outlinks":
		links := make([]any, len(t.Links))
		for i, l := range t.Links {
			links[i] = e.resolveLink(l, n.Path)
		}
		return links, true
	case "task":
		return true, true
	}
	if v, ok := t.Fields.Get(name); ok {
		return e.resolve(v, n.Path), true
	}
	return nil, false
}

// taskObject is a task as a value, e.g. an element of file.tasks.
func (e *executor) taskObject(n *note.Note, t *note.Task) map[string]any {
	obj := make(map[string]any, len(taskFields)+len(t.Fields))
	for _, k := range taskFields {
		obj[k], _ = e.taskField(n, t, k)
	}
	for _, f := range t.Fields {
		obj[f.Key] = e.resolve(f.Value, n.Path)
		if c := note.Canonical(f.Key); c != f.Key {
			obj[c] = obj[f.Key]
		}
	}
	return obj
}

var taskFields = []string{"text", "status", "completed", "checked", "line", "path", "link", "tags", "outlinks", "task"}

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
