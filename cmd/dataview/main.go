// Command dataview runs Dataview (DQL) queries over an Obsidian vault.
//
//	dataview serve                               JSON-RPC 2.0 on stdin/stdout (used by Neovim)
//	dataview query --vault <dir> --dql <query>   one-shot, JSON on stdout
//	dataview version
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/rossop/dataview.nvim/internal/cache"
	"github.com/rossop/dataview.nvim/internal/dql"
	"github.com/rossop/dataview.nvim/internal/exec"
	"github.com/rossop/dataview.nvim/internal/index"
	"github.com/rossop/dataview.nvim/internal/rpc"
	"github.com/rossop/dataview.nvim/internal/scan"
)

// version is set at release time with -ldflags "-X main.version=…".
var version = "dev"

// Exit codes (§I): 0 ok, 1 query or vault error, 2 usage error.
const (
	exitOK    = 0
	exitErr   = 1
	exitUsage = 2
)

func main() {
	// A soft heap limit keeps RSS near the §V.5 budget by making the GC work
	// harder before the process grows. GOMEMLIMIT still overrides it.
	if os.Getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(24 << 20)
	}
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return exitUsage
	}
	switch args[0] {
	case "serve":
		srv := &rpc.Server{Version: version, Log: stderr, UseCache: true}
		if err := srv.Serve(stdin, stdout); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitErr
		}
		return exitOK
	case "query":
		return cmdQuery(args[1:], stdout, stderr)
	case "version", "--version", "-v":
		fmt.Fprintln(stdout, version)
		return exitOK
	case "help", "-h", "--help":
		usage(stdout)
		return exitOK
	}
	fmt.Fprintf(stderr, "error: unknown command %q\n", args[0])
	usage(stderr)
	return exitUsage
}

func usage(w io.Writer) {
	fmt.Fprint(w, `usage:
  dataview serve                               JSON-RPC on stdin/stdout (Neovim)
  dataview query --vault <dir> --dql <query>   run one query, print JSON
  dataview version
`)
}

func cmdQuery(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("query", flag.ContinueOnError)
	fs.SetOutput(stderr)
	vault := fs.String("vault", "", "path to the Obsidian vault")
	query := fs.String("dql", "", "Dataview query, e.g. 'TABLE status FROM #project'")
	noCache := fs.Bool("no-cache", false, "ignore and do not write the index cache")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if *vault == "" || *query == "" {
		fmt.Fprintln(stderr, "error: --vault and --dql are required")
		fs.Usage()
		return exitUsage
	}
	if st, err := os.Stat(*vault); err != nil || !st.IsDir() {
		fmt.Fprintf(stderr, "error: vault %q is not a directory\n", *vault)
		return exitErr
	}

	q, err := dql.Parse(*query)
	if err != nil {
		fmt.Fprintf(stderr, "query error: %v\n", err)
		return exitErr
	}

	ix, err := loadIndex(*vault, !*noCache, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitErr
	}
	res, err := exec.Run(q, ix, exec.Options{})
	if err != nil {
		fmt.Fprintf(stderr, "query error: %v\n", err)
		return exitErr
	}
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(res); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitErr
	}
	return exitOK
}

// loadIndex warms the index from the cache, rescans only what changed, and
// saves the cache back when anything moved.
func loadIndex(vault string, useCache bool, stderr io.Writer) (*index.Index, error) {
	ix := index.New()
	var cachePath string
	if useCache {
		p, err := cache.Path(vault)
		if err == nil {
			cachePath = p
			if err := cache.Load(p, vault, ix); err != nil && !errors.Is(err, cache.ErrStale) {
				return nil, err
			}
		}
	}
	res, err := scan.Vault(vault, ix)
	if err != nil {
		return nil, err
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(stderr, "warning: %s\n", w)
	}
	if cachePath != "" && (res.Updated > 0 || res.Removed > 0) {
		if err := cache.Save(cachePath, vault, ix); err != nil {
			fmt.Fprintf(stderr, "warning: cache not saved: %v\n", err)
		}
	}
	return ix, nil
}
