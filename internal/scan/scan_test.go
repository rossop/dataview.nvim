package scan

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rossop/dataview.nvim/internal/index"
	"github.com/rossop/dataview.nvim/internal/testvault"
)

// copyVault copies the fixture into a temp dir so tests can mutate it
// without touching testdata (§V.2).
func copyVault(t testing.TB) string {
	t.Helper()
	src := filepath.Join("..", "..", "testdata", "vault")
	dst := t.TempDir()
	err := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if info.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

func TestVaultIncremental(t *testing.T) {
	root := copyVault(t)
	ix := index.New()

	res, err := Vault(root, ix)
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated != 5 || ix.Len() != 5 {
		t.Fatalf("first scan: %+v, len %d", res, ix.Len())
	}
	if ix.Get(".obsidian/app.json") != nil {
		t.Error(".obsidian indexed")
	}
	if len(res.Warnings) != 1 {
		t.Errorf("want 1 warning (Broken.md), got %v", res.Warnings)
	}

	res, _ = Vault(root, ix)
	if res.Updated != 0 || res.Unchanged != 5 {
		t.Errorf("rescan should reuse all notes: %+v", res)
	}

	// Change one file (bump mtime explicitly; some file systems have coarse clocks).
	alpha := filepath.Join(root, "Projects", "Alpha.md")
	if err := os.WriteFile(alpha, []byte("#changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Hour)
	os.Chtimes(alpha, future, future)
	os.Remove(filepath.Join(root, "Broken.md"))

	res, _ = Vault(root, ix)
	if res.Updated != 1 || res.Removed != 1 || res.Unchanged != 3 {
		t.Errorf("incremental: %+v", res)
	}
	if got := ix.Get("Projects/Alpha.md").Tags; len(got) != 1 || got[0] != "#changed" {
		t.Errorf("alpha not re-parsed: %v", got)
	}
}

func TestPaths(t *testing.T) {
	root := copyVault(t)
	ix := index.New()
	Vault(root, ix)

	os.WriteFile(filepath.Join(root, "New.md"), []byte("#new"), 0o644)
	os.Remove(filepath.Join(root, "Broken.md"))
	res := Paths(root, []string{"New.md", "Broken.md", "Projects/Beta.md"}, ix)
	if res.Updated != 1 || res.Removed != 1 || res.Unchanged != 1 {
		t.Errorf("paths: %+v", res)
	}
	if ix.Get("New.md") == nil || ix.Get("Broken.md") != nil {
		t.Error("index not updated")
	}
}

func writeSynthetic(tb testing.TB, n int) string {
	tb.Helper()
	root := tb.TempDir()
	if err := testvault.Write(root, n); err != nil {
		tb.Fatal(err)
	}
	return root
}

func BenchmarkScan10k(b *testing.B) {
	root := writeSynthetic(b, 10_000)
	b.ResetTimer()
	for b.Loop() {
		ix := index.New()
		if _, err := Vault(root, ix); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRescan10kUnchanged(b *testing.B) {
	root := writeSynthetic(b, 10_000)
	ix := index.New()
	Vault(root, ix)
	b.ResetTimer()
	for b.Loop() {
		Vault(root, ix)
	}
}
