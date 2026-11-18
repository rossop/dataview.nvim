package cache

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/rossop/dataview.nvim/internal/index"
	"github.com/rossop/dataview.nvim/internal/note"
)

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.gob")
	src := index.New()
	n, _ := note.Parse("A.md", []byte("---\ndue: 2026-12-01\nlist: [1, \"[[B]]\", {k: v}]\nempty:\n---\nk:: [[C]]\n- [ ] t [x:: 1]\n#tag\n"))
	n.MTime = time.Unix(1700000000, 0).UTC()
	src.Put(n)

	if err := Save(path, "/vault", src); err != nil {
		t.Fatal(err)
	}
	dst := index.New()
	if err := Load(path, "/vault", dst); err != nil {
		t.Fatal(err)
	}
	got := dst.Get("A.md")
	if !reflect.DeepEqual(got, n) {
		t.Errorf("round trip mismatch:\n got %#v\nwant %#v", got, n)
	}
}

func TestStale(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.gob")
	ix := index.New()
	ix.Put(&note.Note{Path: "A.md"})
	Save(path, "/vault", ix)

	if err := Load(path, "/other", index.New()); !errors.Is(err, ErrStale) {
		t.Errorf("other vault: %v", err)
	}
	os.WriteFile(path, []byte("garbage"), 0o644)
	fresh := index.New()
	if err := Load(path, "/vault", fresh); !errors.Is(err, ErrStale) || fresh.Len() != 0 {
		t.Errorf("corrupt: %v len %d", err, fresh.Len())
	}
	if err := Load(filepath.Join(dir, "missing"), "/vault", fresh); !errors.Is(err, ErrStale) {
		t.Errorf("missing: %v", err)
	}
}

func TestPathUsesXDG(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/tmp/xdg")
	p, err := Path("/some/vault")
	if err != nil || filepath.Dir(p) != "/tmp/xdg/dataview.nvim" || filepath.Ext(p) != ".gob" {
		t.Errorf("Path = %q, %v", p, err)
	}
}
