# SPEC

## §G GOAL
DQL block in Obsidian note → correct results in Neovim < 100 ms. setup = add plugin, nothing else.

## §C CONSTRAINTS
- Go core (`internal/`) + Lua plugin (`lua/dataview/`). Go stdlib first; dep only w/ reason (YAML parser ok).
- runtime: session process. Neovim spawns `dataview serve` over stdio on first query → exits w/ Neovim. ⊥ socket daemon, ⊥ fsnotify (v1).
- index cached on disk: gob @ `$XDG_CACHE_HOME/dataview.nvim/<vault-hash>.gob`, keyed by mtime+size.
- DQL v1 = core subset: TABLE|LIST|TASK, FROM (folder, #tag, [[link]]), WHERE, SORT, LIMIT, common fns. GROUP BY|FLATTEN|CALENDAR later. ⊥ DataviewJS.
- UI default = native (extmark virt_lines + float). pickers (telescope, fzf-lua, snacks, vim.ui.select) optional adapters only.
- install: prebuilt binary auto-download, SHA256 verified. ⊥ Go toolchain needed by user.
- branch `feat/dvNNNN-*` → merged into `main` locally (no PRs). Conventional Commits, scope = task id (`fix(dv0001): …`).
- green = `just check`.

## §I INTERFACES
- cmd: `dataview serve` → newline-delimited JSON-RPC 2.0 on stdin/stdout
- cmd: `dataview query --vault <dir> --dql <q>` → stdout JSON `Result`; errors → stderr; exit 0 ok, 1 query err, 2 usage err
- rpc: `initialize{vault}` → `{protocolVersion, version, notes}`
- rpc: `query{dql}` → `Result{kind, columns, rows}`
- rpc: `refresh{paths?}` → `{updated, removed}`
- rpc: `shutdown` → `null` → process exits 0
- lua: `require('dataview').setup(opts)`; opts: `vault`, `binary?`, `render = 'inline'|'float'|<picker>`
- nvim cmd: `:Dataview {run|toggle|pick|refresh|log}`; `<Plug>(dataview-*)` maps; ⊥ default keymaps
- health: `:checkhealth dataview`
- file: `testdata/vault/` fixture vault; `justfile` targets `check vet test nvim-test build`

## §V INVARIANTS
V1: ∀ v1 DQL feature → golden test vs Obsidian Dataview output on `testdata/vault/`
V2: vault read-only. ⊥ write|rename|delete under vault path
V3: malformed note (bad YAML etc) → skipped + warning. ⊥ panic, ⊥ scan abort
V4: index stores metadata only. ⊥ note body in memory after parse
V5: RSS ≤ 30 MB while indexing 10k notes
V6: warm query < 50 ms; cold start 10k notes < 1 s w/ cache, < 3 s w/o
V7: scan re-parses file only if mtime|size changed
V8: ∀ Lua→binary call async. ⊥ `vim.fn.system` | blocking wait on UI thread
V9: Lua ⊥ query/parse logic. Lua talks only to §I JSON contract
V10: `internal/{note,index,dql,exec}` ⊥ import os/stdio/flag/rpc. I/O lives in spokes (`scan`, `cache`, `rpc`, `cmd`)
V11: protocol carries `protocolVersion`; Lua mismatch → clear error + `:checkhealth` hint
V12: plugin loads w/ zero plugin deps; optional adapters via `pcall(require)`
V13: cache version mismatch | corrupt → discard + full rescan. ⊥ crash
V14: scan skips `.obsidian/`, `.git/`, non-`.md`
V15: downloaded binary SHA256 ! match release checksum before exec

## §T TASKS
id|status|task|cites
T1|x|dv0001 scaffold: fix `flag.Parse()`, `justfile`, SPEC.md, AGENTS.md, `testdata/vault/`, nvim smoke test|I.cmd,V12
T2|.|dv0002 `internal/note` parser: frontmatter, inline `key:: v` fields (incl `[k:: v]`), tags (nested), wikilinks + md links, tasks|V3,V4
T3|.|dv0003 `internal/index` + `internal/scan` + `internal/cache`; `BenchmarkScan10k`|V5,V6,V7,V13,V14
T4|.|dv0004 `internal/dql` lexer + parser → AST, golden parse tests|V1
T5|.|dv0005 `internal/exec` + `dataview query` one-shot CLI → JSON|V1,V10,I.cmd
T6|.|dv0006 `internal/rpc` + `dataview serve`, pipe integration test|V11,I.rpc
T7|.|dv0007 Lua core: `process`, `blocks`, `render/inline`, `:Dataview run\|toggle`, refresh on `BufWritePost`|V8,V9,V12,I.lua
T8|.|dv0008 `render/float` + `pickers/*` adapters, jump-to-note|V12
T9|.|dv0009 distribution: goreleaser (darwin/linux/windows × amd64/arm64, CGO off, `-s -w`), `install.lua`, `health.lua`, README lazy.nvim spec|V15,I.health
T10|.|dv0010 perf CI: synthetic 10k vault, assert budgets, fail on >20% regress|V5,V6

## §B BUGS
id|date|cause|fix
