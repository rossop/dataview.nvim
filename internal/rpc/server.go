// Package rpc is the editor spoke: newline-delimited JSON-RPC 2.0 over
// stdin/stdout, one request at a time. Neovim starts it lazily and it exits
// with Neovim (§C), so there is no socket and no daemon to manage.
package rpc

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rossop/dataview.nvim/internal/cache"
	"github.com/rossop/dataview.nvim/internal/dql"
	"github.com/rossop/dataview.nvim/internal/exec"
	"github.com/rossop/dataview.nvim/internal/index"
	"github.com/rossop/dataview.nvim/internal/scan"
)

// ProtocolVersion changes only when the wire format breaks; the Lua client
// refuses to talk to a binary with a different value (§V.11).
const ProtocolVersion = 1

// Error codes beyond the JSON-RPC reserved range.
const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeNotInitialized = -32002
	codeQuery          = -32001
)

// staleAfter bounds how old the index may be before a query re-stats the
// vault. Saved buffers are refreshed immediately by the client; this only
// catches edits made outside Neovim (sync, git) without a watcher.
const staleAfter = 5 * time.Second

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type Server struct {
	Version  string
	Log      io.Writer        // warnings; stderr in production
	Now      func() time.Time // injectable for tests
	UseCache bool

	vault    string
	ix       *index.Index
	lastScan time.Time
	dirty    bool
}

type InitializeResult struct {
	ProtocolVersion int    `json:"protocolVersion"`
	Version         string `json:"version"`
	Notes           int    `json:"notes"`
}

// Serve handles requests until EOF or shutdown, then saves the cache.
func (s *Server) Serve(in io.Reader, out io.Writer) error {
	if s.Now == nil {
		s.Now = time.Now
	}
	if s.Log == nil {
		s.Log = io.Discard
	}
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)

	defer s.saveCache()
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			if err := enc.Encode(response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{Code: codeParse, Message: err.Error()}}); err != nil {
				return err
			}
			continue
		}
		result, rerr := s.dispatch(req)
		if req.ID == nil {
			continue // notification: no reply
		}
		resp := response{JSONRPC: "2.0", ID: req.ID, Result: result, Error: rerr}
		if rerr == nil && result == nil {
			resp.Result = json.RawMessage("null")
		}
		if err := enc.Encode(resp); err != nil {
			return err
		}
		if req.Method == "shutdown" {
			return nil
		}
	}
	return sc.Err()
}

func (s *Server) dispatch(req request) (any, *rpcError) {
	if req.JSONRPC != "2.0" || req.Method == "" {
		return nil, &rpcError{Code: codeInvalidRequest, Message: "not a JSON-RPC 2.0 request"}
	}
	switch req.Method {
	case "initialize":
		var p struct {
			Vault string `json:"vault"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.Vault == "" {
			return nil, &rpcError{Code: codeInvalidParams, Message: "initialize needs {vault}"}
		}
		return s.initialize(p.Vault)
	case "shutdown":
		return nil, nil
	}

	if req.Method != "query" && req.Method != "refresh" {
		return nil, &rpcError{Code: codeMethodNotFound, Message: fmt.Sprintf("unknown method %q", req.Method)}
	}
	if s.ix == nil {
		return nil, &rpcError{Code: codeNotInitialized, Message: "call initialize first"}
	}
	switch req.Method {
	case "query":
		var p struct {
			DQL string `json:"dql"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.DQL == "" {
			return nil, &rpcError{Code: codeInvalidParams, Message: "query needs {dql}"}
		}
		return s.query(p.DQL)
	case "refresh":
		var p struct {
			Paths []string `json:"paths"`
		}
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &p); err != nil {
				return nil, &rpcError{Code: codeInvalidParams, Message: err.Error()}
			}
		}
		return s.refresh(p.Paths)
	}
	return nil, nil
}

func (s *Server) initialize(vault string) (any, *rpcError) {
	abs, err := filepath.Abs(vault)
	if err != nil {
		return nil, &rpcError{Code: codeInvalidParams, Message: err.Error()}
	}
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		return nil, &rpcError{Code: codeInvalidParams, Message: fmt.Sprintf("vault %q is not a directory", vault)}
	}
	s.saveCache() // switching vaults: keep the old one's work
	s.vault, s.ix, s.dirty = abs, index.New(), false
	if s.UseCache {
		if p, err := cache.Path(abs); err == nil {
			if err := cache.Load(p, abs, s.ix); err != nil && !errors.Is(err, cache.ErrStale) {
				fmt.Fprintf(s.Log, "warning: %v\n", err)
			}
		}
	}
	if rerr := s.rescan(); rerr != nil {
		s.ix = nil
		return nil, rerr
	}
	s.saveCache()
	return InitializeResult{ProtocolVersion, s.Version, s.ix.Len()}, nil
}

func (s *Server) query(src string) (any, *rpcError) {
	q, err := dql.Parse(src)
	if err != nil {
		e := &rpcError{Code: codeQuery, Message: err.Error()}
		var pe *dql.Error
		if errors.As(err, &pe) {
			e.Data = map[string]int{"line": pe.Line, "col": pe.Col}
		}
		return nil, e
	}
	if s.Now().Sub(s.lastScan) > staleAfter {
		if rerr := s.rescan(); rerr != nil {
			return nil, rerr
		}
	}
	res, err := exec.Run(q, s.ix, exec.Options{Now: s.Now()})
	if err != nil {
		return nil, &rpcError{Code: codeQuery, Message: err.Error()}
	}
	return res, nil
}

func (s *Server) refresh(paths []string) (any, *rpcError) {
	if len(paths) == 0 {
		if rerr := s.rescan(); rerr != nil {
			return nil, rerr
		}
		return map[string]int{"notes": s.ix.Len()}, nil
	}
	rels := make([]string, 0, len(paths))
	for _, p := range paths {
		// Clients send absolute buffer paths; anything outside the vault is ignored.
		if filepath.IsAbs(p) {
			rel, err := filepath.Rel(s.vault, p)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue
			}
			p = rel
		}
		rels = append(rels, p)
	}
	res := scan.Paths(s.vault, rels, s.ix)
	s.logWarnings(res.Warnings)
	if res.Updated > 0 || res.Removed > 0 {
		s.dirty = true
	}
	return res, nil
}

func (s *Server) rescan() *rpcError {
	res, err := scan.Vault(s.vault, s.ix)
	if err != nil {
		return &rpcError{Code: codeInvalidParams, Message: fmt.Sprintf("cannot read vault: %v", err)}
	}
	s.lastScan = s.Now()
	s.logWarnings(res.Warnings)
	if res.Updated > 0 || res.Removed > 0 {
		s.dirty = true
	}
	return nil
}

func (s *Server) logWarnings(ws []string) {
	for _, w := range ws {
		fmt.Fprintf(s.Log, "warning: %s\n", w)
	}
}

func (s *Server) saveCache() {
	if !s.UseCache || !s.dirty || s.ix == nil {
		return
	}
	p, err := cache.Path(s.vault)
	if err == nil {
		err = cache.Save(p, s.vault, s.ix)
	}
	if err != nil {
		fmt.Fprintf(s.Log, "warning: cache not saved: %v\n", err)
		return
	}
	s.dirty = false
}
