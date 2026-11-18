package note

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
)

// TestBodyNotRetained guards §V.4: parsed notes must not keep their source
// text alive through substrings.
func TestBodyNotRetained(t *testing.T) {
	const notes, bodySize = 200, 100_000
	pad := strings.Repeat("x", bodySize)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	kept := make([]*Note, notes)
	for i := range kept {
		src := fmt.Sprintf("k:: v%d\n#t%d [[L%d|d]] [m](M.md)\n- [ ] task [f:: x]\n%s\n", i, i, i, pad)
		kept[i], _ = Parse("n.md", []byte(src))
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(kept)

	retained := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	if limit := int64(notes * bodySize / 10); retained > limit {
		t.Fatalf("retained %d bytes for %d notes (limit %d): bodies leaked", retained, notes, limit)
	}
}
