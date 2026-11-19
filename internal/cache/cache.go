// Package cache persists the index between Neovim sessions so a cold start
// only re-reads files that changed (§V.6). Files live outside the vault.
package cache

import (
	"bufio"
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

// Version must be bumped whenever note.Note, the parser's output or the file
// layout changes, so stale caches are discarded rather than misread (§V.13).
const Version = 2

var ErrStale = errors.New("cache stale or unreadable")

// The file is a header followed by Count notes, each its own gob message.
// Encoding notes one at a time keeps the encoder's buffer at one note
// instead of a full copy of the index (≈14 MB for 10k notes, §V.5).
type header struct {
	Version int
	Vault   string
	Count   int
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
	dec := gob.NewDecoder(bufio.NewReader(f))
	var h header
	if err := dec.Decode(&h); err != nil {
		return fmt.Errorf("%w: %v", ErrStale, err)
	}
	abs, _ := filepath.Abs(vault)
	if h.Version != Version || h.Vault != abs || h.Count < 0 {
		return fmt.Errorf("%w: version %d for %q", ErrStale, h.Version, h.Vault)
	}
	// Decode everything before touching ix, so a truncated file changes nothing.
	notes := make([]*note.Note, 0, h.Count)
	for range h.Count {
		n := new(note.Note)
		if err := dec.Decode(n); err != nil {
			return fmt.Errorf("%w: %v", ErrStale, err)
		}
		notes = append(notes, n)
	}
	for _, n := range notes {
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
	w := bufio.NewWriter(tmp)
	enc := gob.NewEncoder(w)
	notes := ix.All()
	err = enc.Encode(header{Version, abs, len(notes)})
	for _, n := range notes {
		if err != nil {
			break
		}
		err = enc.Encode(n)
	}
	if err == nil {
		err = w.Flush()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
