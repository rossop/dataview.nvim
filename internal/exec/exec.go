// Package exec runs a parsed DQL query against the index. Pure core: the
// clock and time zone come in through Options so results are reproducible.
package exec

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/rossop/dataview.nvim/internal/dql"
	"github.com/rossop/dataview.nvim/internal/index"
	"github.com/rossop/dataview.nvim/internal/note"
)

type Options struct {
	Now time.Time      // for date(today) and friends
	Loc *time.Location // zone for file times; dates are compared as wall-clock values
}

// Result is the JSON contract the editor renders (§I). Cells are display
// text, so the Lua side needs no value logic (§V.9).
type Result struct {
	Kind    string   `json:"kind"` // table | list | task
	Columns []string `json:"columns,omitempty"`
	Rows    []Row    `json:"rows"`
}

type Row struct {
	Path  string    `json:"path"`
	Cells []string  `json:"cells"`
	Task  *TaskInfo `json:"task,omitempty"`
}

type TaskInfo struct {
	Line      int    `json:"line"`
	Status    string `json:"status"`
	Completed bool   `json:"completed"`
}

func Run(q *dql.Query, ix *index.Index, opt Options) (*Result, error) {
	if opt.Loc == nil {
		opt.Loc = time.Local
	}
	if opt.Now.IsZero() {
		opt.Now = time.Now()
	}
	e := &executor{ix: ix, opt: opt, pages: map[string]*pageScope{}}

	var rows []scope
	for _, n := range ix.All() {
		if q.From != nil && !e.matches(q.From, n) {
			continue
		}
		p := e.page(n)
		if q.Kind == dql.Task {
			for i := range n.Tasks {
				rows = append(rows, &taskScope{pg: p, t: &n.Tasks[i]})
			}
		} else {
			rows = append(rows, p)
		}
	}

	for _, c := range q.Commands {
		var err error
		switch c := c.(type) {
		case dql.Where:
			rows, err = e.where(rows, c.Cond)
		case dql.Sort:
			rows, err = e.sort(rows, c.Keys)
		case dql.Limit:
			if c.N < len(rows) {
				rows = rows[:c.N]
			}
		}
		if err != nil {
			return nil, err
		}
	}
	return e.project(q, rows)
}

type executor struct {
	ix    *index.Index
	opt   Options
	pages map[string]*pageScope
	// sets caches FROM link sources so each is computed once per query.
	sets map[dql.LinkSource]map[string]bool
}

func (e *executor) where(rows []scope, cond dql.Expr) ([]scope, error) {
	out := rows[:0]
	for _, r := range rows {
		v, err := e.eval(cond, r)
		if err != nil {
			return nil, fmt.Errorf("WHERE on %s: %w", r.path(), err)
		}
		if truthy(v) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (e *executor) sort(rows []scope, keys []dql.SortKey) ([]scope, error) {
	vals := make([][]any, len(rows))
	for i, r := range rows {
		vals[i] = make([]any, len(keys))
		for k, key := range keys {
			v, err := e.eval(key.Expr, r)
			if err != nil {
				return nil, fmt.Errorf("SORT on %s: %w", r.path(), err)
			}
			vals[i][k] = v
		}
	}
	idx := make([]int, len(rows))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		for k, key := range keys {
			c := compare(vals[idx[a]][k], vals[idx[b]][k])
			if key.Desc {
				c = -c
			}
			if c != 0 {
				return c < 0
			}
		}
		return false
	})
	out := make([]scope, len(rows))
	for i, j := range idx {
		out[i] = rows[j]
	}
	return out, nil
}

func (e *executor) project(q *dql.Query, rows []scope) (*Result, error) {
	res := &Result{Kind: strings.ToLower(string(q.Kind)), Rows: make([]Row, 0, len(rows))}
	if q.Kind == dql.Table {
		if !q.WithoutID {
			res.Columns = append(res.Columns, "File")
		}
		for _, c := range q.Columns {
			res.Columns = append(res.Columns, c.Name)
		}
	}
	for _, r := range rows {
		row := Row{Path: r.path()}
		name := r.page().n.Name()
		switch q.Kind {
		case dql.Table:
			if !q.WithoutID {
				row.Cells = append(row.Cells, name)
			}
			for _, c := range q.Columns {
				v, err := e.eval(c.Expr, r)
				if err != nil {
					return nil, fmt.Errorf("column %q on %s: %w", c.Name, r.path(), err)
				}
				row.Cells = append(row.Cells, display(v))
			}
		case dql.List:
			if q.ListExpr == nil {
				row.Cells = []string{name}
				break
			}
			v, err := e.eval(q.ListExpr, r)
			if err != nil {
				return nil, fmt.Errorf("LIST on %s: %w", r.path(), err)
			}
			if !q.WithoutID {
				row.Cells = append(row.Cells, name)
			}
			row.Cells = append(row.Cells, display(v))
		case dql.Task:
			t := r.(*taskScope).t
			row.Cells = []string{t.Text}
			row.Task = &TaskInfo{Line: t.Line, Status: t.Status, Completed: t.Completed}
		}
		res.Rows = append(res.Rows, row)
	}
	return res, nil
}

// ---- FROM ----

func (e *executor) matches(s dql.Source, n *note.Note) bool {
	switch s := s.(type) {
	case dql.TagSource:
		for _, t := range expandTags(n.Tags) {
			if strings.EqualFold(t, s.Tag) {
				return true
			}
		}
		return false
	case dql.FolderSource:
		if s.Path == "" {
			return true
		}
		p := strings.ToLower(n.Path)
		f := strings.ToLower(strings.TrimSuffix(s.Path, ".md"))
		return strings.HasPrefix(p, f+"/") || p == f+".md"
	case dql.LinkSource:
		return e.linkSet(s)[n.Path]
	case dql.AndSource:
		return e.matches(s.L, n) && e.matches(s.R, n)
	case dql.OrSource:
		return e.matches(s.L, n) || e.matches(s.R, n)
	case dql.NotSource:
		return !e.matches(s.X, n)
	}
	return false
}

func (e *executor) linkSet(s dql.LinkSource) map[string]bool {
	if set, ok := e.sets[s]; ok {
		return set
	}
	set := map[string]bool{}
	if target := e.ix.Resolve(note.Link{Path: s.Path}, ""); target != "" {
		if s.Outgoing {
			if n := e.ix.Get(target); n != nil {
				for _, l := range n.Links {
					if p := e.ix.Resolve(l, n.Path); p != "" {
						set[p] = true
					}
				}
			}
		} else {
			for _, p := range e.ix.Inlinks(target) {
				set[p] = true
			}
		}
	}
	if e.sets == nil {
		e.sets = map[dql.LinkSource]map[string]bool{}
	}
	e.sets[s] = set
	return set
}

// expandTags adds parent tags: #a/b/c → #a, #a/b, #a/b/c (Dataview file.tags).
func expandTags(tags []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, t := range tags {
		parts := strings.Split(strings.TrimPrefix(t, "#"), "/")
		for i := range parts {
			tag := "#" + strings.Join(parts[:i+1], "/")
			if !seen[tag] {
				seen[tag] = true
				out = append(out, tag)
			}
		}
	}
	return out
}
