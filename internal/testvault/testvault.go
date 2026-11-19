// Package testvault writes synthetic vaults for benchmarks and budget tests.
package testvault

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var filler = strings.Repeat("Lorem ipsum dolor sit amet, consectetur adipiscing elit. ", 36) // ≈2 KB

// Write creates n notes under root shaped like real ones: frontmatter,
// inline fields, tags, links and two tasks each, spread over 100 folders.
func Write(root string, n int) error {
	for i := range n {
		dir := filepath.Join(root, fmt.Sprintf("f%02d", i%100))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		body := fmt.Sprintf("---\nstatus: s%d\ntags: [t%d]\ndue: 2026-01-%02d\n---\n# N%d\n\nprio:: %d\nSee [[N%d]] and [[N%d]] #x/y%d\n\n- [ ] task %d [due:: 2026-02-01]\n- [x] done\n%s\n",
			i%5, i%20, i%28+1, i, i%10, (i+1)%n, (i+7)%n, i%3, i, filler)
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("N%d.md", i)), []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}
