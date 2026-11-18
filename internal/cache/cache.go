// Package cache persists the index between Neovim sessions so a cold start
// only re-reads files that changed (§V.6). Files live outside the vault.
package cache

import (
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rossop/dataview.nvim/internal/index"
	"github.com/rossop/dataview.nvim/internal/note"
)

// Version must be bumped whenever note.Note or the parser's output changes,
// so stale caches are discarded rather than misread (§V.13).
const Version = 1

var ErrStale = errors.New("cache stale or unreadable")

type file struct {
	Version int
	Vault   string
	Notes   []*note.Note
}

func init() {
	// Concrete types that can appear inside map[string]any / []any values.
	gob.Register(time.Time{})
	gob.Register(note.Link{})
	gob.Register([]any{})
	gob.Register(map[string]any{})
}

// Path returns the cache file for a vault: $XDG_CACHE_HOME (or the OS cache
// dir)/dataview.nvim/<hash of absolute vault path>.gob.
func Path(vault string) (string, error) {
	abs, err := filepath.Abs(vault)
	if err != nil {
		return "", err
	}
	dir := os.Getenv("XDG_CACHE_HOME")
	if dir == "" {
		if dir, err = os.UserCacheDir(); err != nil {
			return "", err
		}
	}
	sum := sha256.Sum256([]byte(abs))
	return filepath.Join(dir, "dataview.nvim", hex.EncodeToString(sum[:8])+".gob"), nil
}

// Load fills ix from the cache file. Any problem returns ErrStale (wrapped)
// and leaves ix untouched, so the caller just falls back to a full scan.
func Load(path, vault string, ix *index.Index) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrStale, err)
	}
	defer f.Close()
	var c file
	if err := gob.NewDecoder(f).Decode(&c); err != nil {
		return fmt.Errorf("%w: %v", ErrStale, err)
	}
	abs, _ := filepath.Abs(vault)
	if c.Version != Version || c.Vault != abs {
		return fmt.Errorf("%w: version %d for %q", ErrStale, c.Version, c.Vault)
	}
	for _, n := range c.Notes {
		ix.Put(n)
	}
	return nil
}

// Save writes the index atomically (temp file + rename) so a crash never
// leaves a half-written cache.
func Save(path, vault string, ix *index.Index) error {
	abs, err := filepath.Abs(vault)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".dataview-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := gob.NewEncoder(tmp).Encode(file{Version, abs, ix.All()}); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
