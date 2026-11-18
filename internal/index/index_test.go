package index

import (
	"reflect"
	"testing"

	"github.com/rossop/dataview.nvim/internal/note"
)

func put(ix *Index, p string, links ...string) {
	n := &note.Note{Path: p}
	for _, l := range links {
		n.Links = append(n.Links, note.Link{Path: l})
	}
	ix.Put(n)
}

func TestResolve(t *testing.T) {
	ix := New()
	put(ix, "Alpha.md")
	put(ix, "Projects/Alpha.md")
	put(ix, "Projects/Deep/Beta.md")
	put(ix, "Archive/Beta.md")
	cases := []struct{ target, from, want string }{
		{"Alpha", "Other.md", "Alpha.md"},                       // exact root path
		{"Projects/Alpha", "Other.md", "Projects/Alpha.md"},     // exact path
		{"Beta", "Projects/Deep/X.md", "Projects/Deep/Beta.md"}, // same folder wins
		{"Beta", "Other.md", "Archive/Beta.md"},                 // shortest path
		{"deep/beta", "Other.md", "Projects/Deep/Beta.md"},      // suffix, case-insensitive
		{"Missing", "Other.md", ""},
		{"", "Projects/Alpha.md", "Projects/Alpha.md"}, // [[#heading]] is self
	}
	for _, c := range cases {
		if got := ix.Resolve(note.Link{Path: c.target}, c.from); got != c.want {
			t.Errorf("Resolve(%q from %q) = %q, want %q", c.target, c.from, got, c.want)
		}
	}
}

func TestInlinksInvalidate(t *testing.T) {
	ix := New()
	put(ix, "A.md", "B", "B", "A")
	put(ix, "B.md")
	put(ix, "C.md", "B")
	if got := ix.Inlinks("B.md"); !reflect.DeepEqual(got, []string{"A.md", "C.md"}) {
		t.Errorf("inlinks = %v", got)
	}
	ix.Remove("C.md")
	if got := ix.Inlinks("B.md"); !reflect.DeepEqual(got, []string{"A.md"}) {
		t.Errorf("after remove = %v", got)
	}
	if ix.Len() != 2 || ix.All()[0].Path != "A.md" {
		t.Errorf("len/all wrong")
	}
}
