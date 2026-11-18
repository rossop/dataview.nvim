package exec

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rossop/dataview.nvim/internal/dql"
	"github.com/rossop/dataview.nvim/internal/index"
	"github.com/rossop/dataview.nvim/internal/scan"
)

var update = flag.Bool("update", false, "rewrite golden files")

// Fixed clock: Sunday 2026-11-15, so relative dates are reproducible.
var testOpts = Options{Now: time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC), Loc: time.UTC}

func fixtureIndex(t *testing.T) *index.Index {
	t.Helper()
	ix := index.New()
	if _, err := scan.Vault(filepath.Join("..", "..", "testdata", "vault"), ix); err != nil {
		t.Fatal(err)
	}
	return ix
}

func run(t *testing.T, ix *index.Index, src string) (*Result, error) {
	t.Helper()
	q, err := dql.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return Run(q, ix, testOpts)
}

// TestGolden runs testdata/*.dql against testdata/vault. Expected output
// must match Obsidian Dataview on the same vault (§V.1); regenerate with
// -update only after checking the result in Obsidian.
func TestGolden(t *testing.T) {
	ix := fixtureIndex(t)
	files, _ := filepath.Glob(filepath.Join("testdata", "*.dql"))
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".dql")
		t.Run(name, func(t *testing.T) {
			src, _ := os.ReadFile(f)
			res, err := run(t, ix, string(src))
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			got, _ := json.MarshalIndent(res, "", "  ")
			got = append(got, '\n')
			golden := strings.TrimSuffix(f, ".dql") + ".golden.json"
			if *update {
				os.WriteFile(golden, got, 0o644)
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("missing golden (run with -update): %v", err)
			}
			if string(got) != string(want) {
				t.Errorf("result mismatch for %s\n got:\n%s\nwant:\n%s", src, got, want)
			}
		})
	}
}

func TestErrors(t *testing.T) {
	ix := fixtureIndex(t)
	for _, c := range []struct{ src, want string }{
		{`LIST WHERE priority - "a"`, "no '-' for number and string"},
		{`LIST WHERE nosuch(1)`, "unknown function nosuch()"},
		{`LIST WHERE contains(file.tags)`, "contains() needs 2 argument(s)"},
		{`TABLE regextest("(", file.name)`, "regextest()"},
	} {
		if _, err := run(t, ix, c.src); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want containing %q", c.src, err, c.want)
		}
	}
}

func TestValues(t *testing.T) {
	ix := fixtureIndex(t)
	cases := []struct{ expr, want string }{
		{`1 + 2 * 3`, "7"},
		{`7 % 4`, "3"},
		{`10 / 4`, "2.5"},
		{`1 / 0`, "-"},
		{`"a" + 1`, "a1"},
		{`null + 1`, "-"},
		{`date(2026-01-31) + dur(1 month)`, "February 28, 2026"},
		{`date(2026-03-01) - date(2026-02-01)`, "4 weeks"},
		{`dur(1 week) + dur(2 days, 3 hours)`, "1 week, 2 days, 3 hours"},
		{`dur(1 day) * 3`, "3 days"},
		{`date(sow)`, "November 09, 2026"},
		{`date(eom) > date(2026-11-30)`, "true"},
		{`date(today).weekday`, "7"},
		{`date(2026-03-31) - dur(1 month)`, "February 28, 2026"},
		{`round(2.345, 2)`, "2.35"},
		{`min(3, 1, 2)`, "1"},
		{`sum([1, 2, 3])`, "6"},
		{`average([])`, "-"},
		{`join(reverse(sort(["b", "a", "c"])), "")`, "cba"},
		{`default(nothing, "x")`, "x"},
		{`choice(1 > 2, "y", "n")`, "n"},
		{`[[Alpha]].status`, "active"},
		{`[[Alpha]] = "Alpha"`, "true"},
		{`econtains(["word"], "wor")`, "false"},
		{`contains(["word"], "wor")`, "true"},
		{`icontains("Hello", "LL")`, "true"},
		{`regexmatch("a.c", "abc")`, "true"},
		{`split("a, b,c", ", ?")`, "a, b, c"},
		{`number("about 12.5 km")`, "12.5"},
		{`typeof(dur(1 s))`, "duration"},
		{`upper("x") + lower("Y")`, "Xy"},
		{`[1, 2, 3][-1]`, "3"},
		{`!true or (1 < 2 and "b" > "a")`, "true"},
		{`dateformat(date(2026-11-05T14:07), "EEE d MMM yyyy HH:mm")`, "Thu 5 Nov 2026 14:07"},
	}
	for _, c := range cases {
		res, err := run(t, ix, "TABLE WITHOUT ID "+c.expr+` WHERE file.name = "Alpha"`)
		if err != nil {
			t.Errorf("%s: %v", c.expr, err)
			continue
		}
		if got := res.Rows[0].Cells[0]; got != c.want {
			t.Errorf("%s = %q, want %q", c.expr, got, c.want)
		}
	}
}
