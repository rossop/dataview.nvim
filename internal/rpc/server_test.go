package rpc

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// client drives a Server over in-memory pipes, as Neovim does over stdio.
type client struct {
	t    *testing.T
	in   *io.PipeWriter
	out  *bufio.Scanner
	done chan error
	id   int
}

func start(t *testing.T, s *Server) *client {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	c := &client{t: t, in: inW, out: bufio.NewScanner(outR), done: make(chan error, 1)}
	go func() {
		c.done <- s.Serve(inR, outW)
		outW.Close()
	}()
	t.Cleanup(func() { inW.Close() })
	return c
}

type reply struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

func (c *client) call(method string, params any) reply {
	c.t.Helper()
	c.id++
	msg, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": c.id, "method": method, "params": params})
	c.send(string(msg))
	return c.recv()
}

func (c *client) send(line string) {
	c.t.Helper()
	if _, err := io.WriteString(c.in, line+"\n"); err != nil {
		c.t.Fatal(err)
	}
}

func (c *client) recv() reply {
	c.t.Helper()
	if !c.out.Scan() {
		c.t.Fatalf("no reply: %v", c.out.Err())
	}
	var r reply
	if err := json.Unmarshal(c.out.Bytes(), &r); err != nil {
		c.t.Fatalf("bad reply %q: %v", c.out.Text(), err)
	}
	return r
}

func copyVault(t *testing.T) string {
	t.Helper()
	src := filepath.Join("..", "..", "testdata", "vault")
	dst := t.TempDir()
	filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		rel, _ := filepath.Rel(src, p)
		if info.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		b, _ := os.ReadFile(p)
		return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
	})
	return dst
}

func TestSession(t *testing.T) {
	vault := copyVault(t)
	clock := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	c := start(t, &Server{Version: "test", Now: func() time.Time { return clock }})

	if r := c.call("query", map[string]string{"dql": "LIST"}); r.Error == nil || r.Error.Code != codeNotInitialized {
		t.Fatalf("query before initialize: %+v", r)
	}

	r := c.call("initialize", map[string]string{"vault": vault})
	var init InitializeResult
	json.Unmarshal(r.Result, &init)
	if r.Error != nil || init.ProtocolVersion != ProtocolVersion || init.Notes != 5 || init.Version != "test" {
		t.Fatalf("initialize: %+v %s", r.Error, r.Result)
	}

	r = c.call("query", map[string]string{"dql": `LIST FROM "Projects"`})
	if r.Error != nil || !strings.Contains(string(r.Result), `"Projects/Beta.md"`) {
		t.Fatalf("query: %+v %s", r.Error, r.Result)
	}

	// A saved buffer is picked up immediately via refresh, even though the
	// stale-index timer has not fired.
	newNote := filepath.Join(vault, "Projects", "Gamma.md")
	os.WriteFile(newNote, []byte("#project\n"), 0o644)
	r = c.call("refresh", map[string]any{"paths": []string{newNote, "/outside/vault.md"}})
	if r.Error != nil || !strings.Contains(string(r.Result), `"updated":1`) {
		t.Fatalf("refresh: %+v %s", r.Error, r.Result)
	}
	r = c.call("query", map[string]string{"dql": `LIST FROM "Projects"`})
	if !strings.Contains(string(r.Result), "Gamma") {
		t.Errorf("refresh not visible: %s", r.Result)
	}

	// External edits appear once the index is older than staleAfter.
	os.WriteFile(filepath.Join(vault, "Projects", "Delta.md"), []byte("x"), 0o644)
	r = c.call("query", map[string]string{"dql": `LIST FROM "Projects"`})
	if strings.Contains(string(r.Result), "Delta") {
		t.Errorf("rescanned before staleAfter: %s", r.Result)
	}
	clock = clock.Add(staleAfter + time.Second)
	r = c.call("query", map[string]string{"dql": `LIST FROM "Projects"`})
	if !strings.Contains(string(r.Result), "Delta") {
		t.Errorf("not rescanned after staleAfter: %s", r.Result)
	}

	r = c.call("query", map[string]string{"dql": "LIST WHERE (a"})
	if r.Error == nil || r.Error.Code != codeQuery || !strings.Contains(r.Error.Message, `expected ")"`) {
		t.Errorf("parse error: %+v", r.Error)
	}

	r = c.call("shutdown", nil)
	if r.Error != nil || string(r.Result) != "null" {
		t.Errorf("shutdown: %+v %s", r.Error, r.Result)
	}
	if err := <-c.done; err != nil {
		t.Errorf("serve returned %v", err)
	}
}

func TestProtocolErrors(t *testing.T) {
	c := start(t, &Server{})
	c.send("{not json")
	if r := c.recv(); r.Error == nil || r.Error.Code != codeParse {
		t.Errorf("bad json: %+v", r)
	}
	if r := c.call("nope", nil); r.Error == nil || r.Error.Code != codeMethodNotFound {
		t.Errorf("unknown method: %+v", r)
	}
	if r := c.call("initialize", map[string]string{}); r.Error == nil || r.Error.Code != codeInvalidParams {
		t.Errorf("missing vault: %+v", r)
	}
	if r := c.call("initialize", map[string]string{"vault": "/does/not/exist"}); r.Error == nil || !strings.Contains(r.Error.Message, "not a directory") {
		t.Errorf("bad vault: %+v", r)
	}
	// Notifications (no id) get no reply; the next call's reply must be its own.
	c.send(`{"jsonrpc":"2.0","method":"refresh"}`)
	if r := c.call("shutdown", nil); r.ID != c.id {
		t.Errorf("got reply for id %d, want %d", r.ID, c.id)
	}
}

func TestCacheSavedOnShutdown(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheDir)
	c := start(t, &Server{UseCache: true})
	c.call("initialize", map[string]string{"vault": copyVault(t)})
	c.call("shutdown", nil)
	<-c.done
	if files, _ := filepath.Glob(filepath.Join(cacheDir, "dataview.nvim", "*.gob")); len(files) != 1 {
		t.Errorf("cache files: %v", files)
	}
}
