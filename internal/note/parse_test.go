package note

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func date(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

func TestParseValue(t *testing.T) {
	cases := []struct {
		in   string
		want any
	}{
		{"", nil},
		{"  42 ", 42.0},
		{"-3.5", -3.5},
		{"TRUE", true},
		{"false", false},
		{"2026-11-30", date("2026-11-30")},
		{"2026-11-30T09:15", time.Date(2026, 11, 30, 9, 15, 0, 0, time.UTC)},
		{"[[Beta]]", Link{Path: "Beta"}},
		{"[[Beta#Sec|b]]", Link{Path: "Beta", Subpath: "Sec", Display: "b"}},
		{"hello world", "hello world"},
		{"12 apples", "12 apples"},
	}
	for _, c := range cases {
		if got := ParseValue(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("ParseValue(%q) = %#v, want %#v", c.in, got, c.want)
		}
	}
}

func TestInlineFields(t *testing.T) {
	src := strings.Join([]string{
		"Owner:: Peter",
		"- **Status**:: open",
		"Text with [rating:: 4] and (mood:: good) inline.",
		"repeat:: a",
		"repeat:: b",
		"Not a field: single colon",
		"http://example.com:: is not a key",
	}, "\n")
	n, warns := Parse("a.md", []byte(src))
	if len(warns) != 0 {
		t.Fatalf("unexpected warnings: %v", warns)
	}
	want := map[string]any{
		"Owner":  "Peter",
		"Status": "open",
		"rating": 4.0,
		"mood":   "good",
		"repeat": []any{"a", "b"},
	}
	if !reflect.DeepEqual(n.Fields, want) {
		t.Errorf("Fields = %#v\nwant %#v", n.Fields, want)
	}
}

func TestTags(t *testing.T) {
	src := "#top and #nested/child, (#paren) not#inline #123 #a1 `#code`\n# Heading\n"
	n, _ := Parse("a.md", []byte(src))
	want := []string{"#top", "#nested/child", "#paren", "#a1"}
	if !reflect.DeepEqual(n.Tags, want) {
		t.Errorf("Tags = %v, want %v", n.Tags, want)
	}
}

func TestLinks(t *testing.T) {
	src := "See [[Beta]], ![[img.png]], [[Areas/Reading#Ch 1|r]]\n" +
		"[md](../Other%20Note.md) [web](https://x.y) [anchor](#h) [pdf](a.pdf)\n"
	n, _ := Parse("Projects/Alpha.md", []byte(src))
	want := []Link{
		{Path: "Beta"},
		{Path: "img.png", Embed: true},
		{Path: "Areas/Reading", Subpath: "Ch 1", Display: "r"},
		{Path: "Other Note", Display: "md"},
	}
	if !reflect.DeepEqual(n.Links, want) {
		t.Errorf("Links = %#v\nwant %#v", n.Links, want)
	}
}

func TestTasks(t *testing.T) {
	src := "intro\n- [ ] open #t [due:: 2026-11-30]\n  * [x] done\n1. [-] cancelled\n- not a task\n"
	n, _ := Parse("a.md", []byte(src))
	if len(n.Tasks) != 3 {
		t.Fatalf("got %d tasks, want 3: %#v", len(n.Tasks), n.Tasks)
	}
	first := n.Tasks[0]
	if first.Line != 1 || first.Completed || first.Status != " " || first.Text != "open #t [due:: 2026-11-30]" {
		t.Errorf("task 0 = %#v", first)
	}
	if !reflect.DeepEqual(first.Tags, []string{"#t"}) || !reflect.DeepEqual(first.Fields, map[string]any{"due": date("2026-11-30")}) {
		t.Errorf("task 0 meta = %v %v", first.Tags, first.Fields)
	}
	if !n.Tasks[1].Completed || n.Tasks[1].Line != 2 {
		t.Errorf("task 1 = %#v", n.Tasks[1])
	}
	if n.Tasks[2].Status != "-" || n.Tasks[2].Completed {
		t.Errorf("task 2 = %#v", n.Tasks[2])
	}
	// Task-line fields also surface on the page, as in Dataview.
	if n.Fields["due"] == nil {
		t.Errorf("page fields missing task field: %v", n.Fields)
	}
}

func TestFencedCodeIgnored(t *testing.T) {
	src := "```\n#nottag [[NotLink]] k:: v\n- [ ] not a task\n```\n~~~js\n#no\n~~~\n#yes\n"
	n, _ := Parse("a.md", []byte(src))
	if !reflect.DeepEqual(n.Tags, []string{"#yes"}) || n.Links != nil || n.Fields != nil || n.Tasks != nil {
		t.Errorf("code leaked: tags=%v links=%v fields=%v tasks=%v", n.Tags, n.Links, n.Fields, n.Tasks)
	}
}

func TestFrontmatter(t *testing.T) {
	src := "---\ntitle: X\ncount: 3\ndue: 2026-12-01\nup: \"[[Home]]\"\ntags: [a, \"#b/c\"]\naliases: one\n---\n" +
		"---\nnot: frontmatter\n---\n"
	n, warns := Parse("a.md", []byte(src))
	if len(warns) != 0 {
		t.Fatalf("warnings: %v", warns)
	}
	if n.Frontmatter["count"] != 3.0 || n.Frontmatter["title"] != "X" {
		t.Errorf("scalars: %#v", n.Frontmatter)
	}
	if !reflect.DeepEqual(n.Frontmatter["due"], date("2026-12-01")) {
		t.Errorf("due = %#v", n.Frontmatter["due"])
	}
	if !reflect.DeepEqual(n.Tags, []string{"#a", "#b/c"}) || !reflect.DeepEqual(n.Aliases, []string{"one"}) {
		t.Errorf("tags=%v aliases=%v", n.Tags, n.Aliases)
	}
	if !reflect.DeepEqual(n.Links, []Link{{Path: "Home"}}) {
		t.Errorf("frontmatter links = %#v", n.Links)
	}
	// The second --- block is body text, so "not:: frontmatter" style data must not appear.
	if _, ok := n.Frontmatter["not"]; ok {
		t.Error("second block parsed as frontmatter")
	}
}

func TestMalformedFrontmatter(t *testing.T) {
	for _, src := range []string{
		"---\nstatus: [unclosed\n---\n#ok\n",
		"---\n: : :\n\t- bad\n---\n",
		"---\nno closing fence\n",
		"",
		"\x00\xff\xfe binary-ish",
	} {
		n, _ := Parse("bad.md", []byte(src))
		if n == nil {
			t.Fatalf("nil note for %q", src)
		}
	}
	n, warns := Parse("bad.md", []byte("---\nstatus: [unclosed\n---\n#ok\n"))
	if len(warns) != 1 || n.Frontmatter != nil || !reflect.DeepEqual(n.Tags, []string{"#ok"}) {
		t.Errorf("warns=%v fm=%v tags=%v", warns, n.Frontmatter, n.Tags)
	}
}

func TestNameFolder(t *testing.T) {
	n := &Note{Path: "Projects/Sub/My Note.md"}
	if n.Name() != "My Note" || n.Folder() != "Projects/Sub" {
		t.Errorf("name=%q folder=%q", n.Name(), n.Folder())
	}
	if (&Note{Path: "Root.md"}).Folder() != "" {
		t.Error("root folder should be empty")
	}
}

// TestFixtureVault parses every note in testdata/vault so a fixture edit that
// breaks the parser is caught here, not in the executor's golden tests.
func TestFixtureVault(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "vault")
	read := func(rel string) *Note {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		n, _ := Parse(rel, b)
		return n
	}

	alpha := read("Projects/Alpha.md")
	if alpha.Frontmatter["status"] != "active" || alpha.Fields["priority"] != 1.0 || alpha.Fields["Owner"] != "Peter" {
		t.Errorf("alpha fm=%v fields=%v", alpha.Frontmatter, alpha.Fields)
	}
	if !reflect.DeepEqual(alpha.Tags, []string{"#project", "#work/research", "#focus"}) {
		t.Errorf("alpha tags = %v", alpha.Tags)
	}
	if len(alpha.Tasks) != 2 || alpha.Tasks[1].Completed != true {
		t.Errorf("alpha tasks = %#v", alpha.Tasks)
	}

	beta := read("Projects/Beta.md")
	wantLinks := []Link{{Path: "Alpha"}, {Path: "Areas/Reading", Display: "an external doc"}}
	if !reflect.DeepEqual(beta.Links, wantLinks) {
		t.Errorf("beta links = %#v", beta.Links)
	}

	reading := read("Areas/Reading.md")
	if reading.Frontmatter != nil || reading.Fields["rating"] != 4.0 || !reflect.DeepEqual(reading.Tags, []string{"#book/nonfiction"}) {
		t.Errorf("reading = %#v", reading)
	}

	if b := read("Broken.md"); b.Frontmatter != nil {
		t.Errorf("broken frontmatter should be dropped: %v", b.Frontmatter)
	}
}

// FuzzParse guards §V.3: no input may panic the parser.
func FuzzParse(f *testing.F) {
	for _, s := range []string{"---\na: 1\n---\n", "- [ ] t [k:: v] #x [[l|d]]", "```\n", "[a](b%zz.md)"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		Parse("f.md", []byte(s))
	})
}
