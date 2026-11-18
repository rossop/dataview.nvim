// Package note turns the bytes of one Markdown file into the metadata Dataview
// queries need. It is part of the pure core: no file system, no clock (§V.10).
// Only metadata is kept; the body is discarded after parsing (§V.4).
package note

import (
	"path"
	"strings"
	"time"
)

// Link is an outgoing link as written in the note. Wikilink targets are left
// unresolved (resolution needs the whole vault, so the index does it);
// Markdown-link targets are made vault-relative because they are path-based.
type Link struct {
	Path    string `json:"path"`
	Subpath string `json:"subpath,omitempty"`
	Display string `json:"display,omitempty"`
	Embed   bool   `json:"embed,omitempty"`
}

type Task struct {
	Line      int            `json:"line"` // 0-based, like Dataview
	Text      string         `json:"text"`
	Status    string         `json:"status"`
	Completed bool           `json:"completed"`
	Tags      []string       `json:"tags,omitempty"`
	Links     []Link         `json:"links,omitempty"`
	Fields    map[string]any `json:"fields,omitempty"`
}

// Note is everything the index keeps about one file. Values in Frontmatter and
// Fields are normalised to: nil, bool, float64, string, time.Time, Link,
// []any, map[string]any.
type Note struct {
	Path        string // vault-relative, forward slashes, with .md
	Size        int64
	MTime       time.Time
	CTime       time.Time
	Frontmatter map[string]any
	Fields      map[string]any // inline fields, including those on task lines
	Tags        []string       // explicit tags, "#" prefixed, deduped, in order
	Aliases     []string
	Links       []Link
	Tasks       []Task
}

// Name is the file name without extension, as Dataview's file.name.
func (n *Note) Name() string {
	return strings.TrimSuffix(path.Base(n.Path), path.Ext(n.Path))
}

// Folder is the vault-relative parent folder, "" for the vault root.
func (n *Note) Folder() string {
	d := path.Dir(n.Path)
	if d == "." {
		return ""
	}
	return d
}
