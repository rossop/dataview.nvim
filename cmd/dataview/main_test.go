package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const vault = "../../testdata/vault"

func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(args, strings.NewReader(""), &out, &errb)
	return code, out.String(), errb.String()
}

func TestQueryJSON(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	code, out, _ := runCLI(t, "query", "--vault", vault, "--dql", `LIST FROM "Projects"`)
	if code != exitOK {
		t.Fatalf("exit %d", code)
	}
	var res struct {
		Kind string
		Rows []struct{ Path string }
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("bad JSON %q: %v", out, err)
	}
	if res.Kind != "list" || len(res.Rows) != 2 || res.Rows[0].Path != "Projects/Alpha.md" {
		t.Errorf("unexpected result: %s", out)
	}
}

func TestCacheWrittenOutsideVault(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheDir)
	before := snapshot(t, vault)
	for range 2 { // second run loads the cache
		if code, _, stderr := runCLI(t, "query", "--vault", vault, "--dql", "LIST"); code != exitOK {
			t.Fatalf("exit %d: %s", code, stderr)
		}
	}
	files, _ := filepath.Glob(filepath.Join(cacheDir, "dataview.nvim", "*.gob"))
	if len(files) != 1 {
		t.Errorf("want one cache file, got %v", files)
	}
	if after := snapshot(t, vault); after != before {
		t.Error("vault changed during query (§V.2)")
	}
}

func snapshot(t *testing.T, root string) string {
	var b strings.Builder
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil {
			b.WriteString(p + info.ModTime().String() + "\n")
		}
		return nil
	})
	return b.String()
}

func TestExitCodes(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	cases := []struct {
		args []string
		code int
		err  string
	}{
		{nil, exitUsage, "usage"},
		{[]string{"bogus"}, exitUsage, `unknown command "bogus"`},
		{[]string{"query", "--vault", vault}, exitUsage, "--vault and --dql are required"},
		{[]string{"query", "--vault", "/nonexistent", "--dql", "LIST"}, exitErr, "is not a directory"},
		{[]string{"query", "--vault", vault, "--dql", "LIST GROUP BY x"}, exitErr, "GROUP is not supported"},
		{[]string{"query", "--vault", vault, "--dql", "LIST WHERE nope()"}, exitErr, "unknown function"},
	}
	for _, c := range cases {
		code, _, stderr := runCLI(t, c.args...)
		if code != c.code || !strings.Contains(stderr, c.err) {
			t.Errorf("%v: exit %d stderr %q, want %d containing %q", c.args, code, stderr, c.code, c.err)
		}
	}
	if code, out, _ := runCLI(t, "version"); code != exitOK || strings.TrimSpace(out) != "dev" {
		t.Errorf("version: %d %q", code, out)
	}
}
