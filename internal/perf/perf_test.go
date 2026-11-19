//go:build !windows

// Package perf asserts the §V.5/§V.6 budgets against the real binary on a
// synthetic 10k-note vault. Slow (≈10 s), so it only runs with
// DATAVIEW_PERF=1: `just perf`, and in CI.
package perf

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"syscall"
	"testing"
	"time"

	"github.com/rossop/dataview.nvim/internal/testvault"
)

const notes = 10_000

// Budgets from SPEC §V. CI runners are slower and noisier than a laptop, so
// timing budgets get PERF_SLACK (default 1.0) as a multiplier.
const (
	budgetRSS       = 30 << 20
	budgetWarmQuery = 50 * time.Millisecond
	budgetColdCache = 1 * time.Second
	budgetColdNone  = 3 * time.Second
)

type session struct {
	cmd *exec.Cmd
	in  io.WriteCloser
	out *bufio.Scanner
	id  int
}

func startServe(t *testing.T, bin, cacheDir string) *session {
	t.Helper()
	cmd := exec.Command(bin, "serve")
	cmd.Env = append(os.Environ(), "XDG_CACHE_HOME="+cacheDir)
	in, _ := cmd.StdinPipe()
	out, _ := cmd.StdoutPipe()
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	return &session{cmd: cmd, in: in, out: sc}
}

func (s *session) call(t *testing.T, method string, params any) (json.RawMessage, time.Duration) {
	t.Helper()
	s.id++
	msg, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": s.id, "method": method, "params": params})
	start := time.Now()
	if _, err := s.in.Write(append(msg, '\n')); err != nil {
		t.Fatal(err)
	}
	if !s.out.Scan() {
		t.Fatalf("%s: no reply: %v", method, s.out.Err())
	}
	elapsed := time.Since(start)
	var r struct {
		Result json.RawMessage
		Error  *struct{ Message string }
	}
	if err := json.Unmarshal(s.out.Bytes(), &r); err != nil || r.Error != nil {
		t.Fatalf("%s failed: %v %+v", method, err, r.Error)
	}
	return r.Result, elapsed
}

// close shuts the server down and returns its peak RSS in bytes.
func (s *session) close(t *testing.T) int64 {
	t.Helper()
	s.call(t, "shutdown", nil)
	s.in.Close()
	if err := s.cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	ru, ok := s.cmd.ProcessState.SysUsage().(*syscall.Rusage)
	if !ok {
		return -1
	}
	if runtime.GOOS == "darwin" {
		return int64(ru.Maxrss) // bytes on macOS
	}
	return int64(ru.Maxrss) << 10 // KiB on Linux
}

func slack() float64 {
	var f float64
	if _, err := fmt.Sscan(os.Getenv("PERF_SLACK"), &f); err == nil && f > 0 {
		return f
	}
	return 1
}

func within(d, budget time.Duration) bool {
	return d <= time.Duration(float64(budget)*slack())
}

func TestBudgets(t *testing.T) {
	if os.Getenv("DATAVIEW_PERF") == "" {
		t.Skip("set DATAVIEW_PERF=1 (or run `just perf`)")
	}
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "dataview")
	build := exec.Command("go", "build", "-o", bin, "../../cmd/dataview")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	vault := filepath.Join(tmp, "vault")
	if err := testvault.Write(vault, notes); err != nil {
		t.Fatal(err)
	}
	cacheDir := filepath.Join(tmp, "cache")
	init := map[string]string{"vault": vault}

	// Cold, no cache: full parse of every note.
	s := startServe(t, bin, cacheDir)
	_, coldNone := s.call(t, "initialize", init)
	rssIndex := s.close(t)

	// Cold with cache: a new session reuses the saved index.
	s = startServe(t, bin, cacheDir)
	_, coldCache := s.call(t, "initialize", init)

	queries := []string{
		`TABLE status, due, prio FROM #t3 WHERE prio > 3 SORT due DESC LIMIT 20`,
		`LIST FROM "f42" WHERE contains(file.outlinks, [[N4207]])`,
		`TASK WHERE !completed AND due <= date(2026-02-01) LIMIT 50`,
		`TABLE length(file.inlinks) AS refs SORT refs DESC LIMIT 10`,
	}
	for _, q := range queries {
		s.call(t, "query", map[string]string{"dql": q}) // first run builds inlinks
		var times []time.Duration
		for range 7 {
			_, d := s.call(t, "query", map[string]string{"dql": q})
			times = append(times, d)
		}
		sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
		median := times[len(times)/2]
		t.Logf("warm query %-70q median %v", q, median.Round(100*time.Microsecond))
		if !within(median, budgetWarmQuery) {
			t.Errorf("V6: warm query %q took %v, budget %v", q, median, budgetWarmQuery)
		}
	}
	rssQuery := s.close(t)

	t.Logf("cold start, no cache:   %v (budget %v)", coldNone.Round(time.Millisecond), budgetColdNone)
	t.Logf("cold start, with cache: %v (budget %v)", coldCache.Round(time.Millisecond), budgetColdCache)
	t.Logf("peak RSS indexing: %.1f MB, querying: %.1f MB (budget %d MB)", float64(rssIndex)/(1<<20), float64(rssQuery)/(1<<20), budgetRSS>>20)

	if !within(coldNone, budgetColdNone) {
		t.Errorf("V6: cold start without cache %v, budget %v", coldNone, budgetColdNone)
	}
	if !within(coldCache, budgetColdCache) {
		t.Errorf("V6: cold start with cache %v, budget %v", coldCache, budgetColdCache)
	}
	if rssIndex > budgetRSS {
		t.Errorf("V5: peak RSS while indexing %.1f MB, budget %d MB", float64(rssIndex)/(1<<20), budgetRSS>>20)
	}
	if rssQuery > budgetRSS {
		t.Errorf("V5: peak RSS while querying %.1f MB, budget %d MB", float64(rssQuery)/(1<<20), budgetRSS>>20)
	}
}
