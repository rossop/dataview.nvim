// Package archtest enforces the hub-and-spokes boundary (§V.10): the core
// packages must not do I/O or depend on the spokes.
package archtest

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var core = []string{"note", "index", "dql", "exec"}

var forbidden = []string{
	"os", "os/exec", "io/fs", "io/ioutil", "net", "net/http", "syscall", "flag", "log",
	"github.com/rossop/dataview.nvim/internal/scan",
	"github.com/rossop/dataview.nvim/internal/cache",
	"github.com/rossop/dataview.nvim/internal/rpc",
}

func TestCoreHasNoIO(t *testing.T) {
	for _, pkg := range core {
		files, _ := filepath.Glob(filepath.Join("..", pkg, "*.go"))
		if len(files) == 0 {
			t.Fatalf("no files for core package %s", pkg)
		}
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			af, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, imp := range af.Imports {
				path, _ := strconv.Unquote(imp.Path.Value)
				for _, bad := range forbidden {
					if path == bad {
						t.Errorf("%s imports %q: core packages must stay I/O-free (§V.10)", f, path)
					}
				}
			}
		}
	}
}
