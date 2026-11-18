package dql

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

// TestGolden parses every testdata/parse/*.dql and compares the AST dump with
// the matching .golden file. Run `go test ./internal/dql -update` after an
// intended change, then review the diff.
func TestGolden(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("testdata", "parse", "*.dql"))
	if len(files) == 0 {
		t.Fatal("no golden inputs")
	}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".dql")
		t.Run(name, func(t *testing.T) {
			src, _ := os.ReadFile(f)
			q, err := Parse(string(src))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			got := Dump(q) + "\n"
			golden := strings.TrimSuffix(f, ".dql") + ".golden"
			if *update {
				os.WriteFile(golden, []byte(got), 0o644)
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("missing golden (run with -update): %v", err)
			}
			if got != string(want) {
				t.Errorf("AST mismatch\n got:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{"", "1:1: expected TABLE, LIST or TASK, found end of query"},
		{"SELECT x", "1:1: expected TABLE, LIST or TASK"},
		{"TABLE a\nGROUP BY b", "2:1: GROUP is not supported yet"},
		{"LIST FLATTEN x", "1:6: FLATTEN is not supported yet"},
		{"CALENDAR file.day", "CALENDAR queries are not supported yet"},
		{`LIST WHERE "open`, "1:12: unterminated string"},
		{"LIST WHERE a = ", "expected an expression, found end of query"},
		{"LIST FROM", "expected a source"},
		{"LIST LIMIT x", "LIMIT needs a number"},
		{"LIST WHERE a SORT b FROM #x", "FROM must come before"},
		{"TABLE (a", `expected ")"`},
		{"LIST WHERE a ~ b", "unexpected character '~'"},
		{"LIST WITHOUT", "expected ID after WITHOUT"},
	}
	for _, c := range cases {
		_, err := Parse(c.src)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("Parse(%q) error = %v, want containing %q", c.src, err, c.want)
		}
	}
}

func FuzzParse(f *testing.F) {
	files, _ := filepath.Glob(filepath.Join("testdata", "parse", "*.dql"))
	for _, p := range files {
		b, _ := os.ReadFile(p)
		f.Add(string(b))
	}
	f.Fuzz(func(t *testing.T, s string) {
		Parse(s)
	})
}
