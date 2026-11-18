# dataview.nvim

Run Obsidian Dataview (DQL) queries from Neovim. A Go binary does the parsing, indexing and querying; a thin Lua plugin finds query blocks and shows results. `SPEC.md` is the source of truth: goals (§G), constraints (§C), interfaces (§I), invariants (§V) and tasks (§T). Read it before changing anything.

## Hard rules
- Every change must keep the §V invariants true. If a change needs one relaxed, amend `SPEC.md` first (via `/spec amend`), don't just break it.
- Hub and spokes: `internal/{note,index,dql,exec}` is the pure core and must not do I/O. File system, cache, RPC and flags live in the spokes (`internal/{scan,cache,rpc}`, `cmd/`). Lua contains no query logic and only speaks the JSON contract in §I.
- Budgets are requirements, not goals: ≤ 30 MB RSS and < 50 ms warm query (§V.5, §V.6). Don't add a dependency, goroutine pool or cache without checking them.
- The vault is read-only. Never write under it, including in tests (copy `testdata/vault/` to a temp dir if a test needs to mutate).
- Neovim must never block: every call to the binary is async.
- No hard plugin dependencies. Telescope, fzf-lua and snacks are optional adapters loaded with `pcall(require)`.

## Workflow
- One §T task per branch: `feat/dvNNNN-<slug>` (or `fix/…`), PR into `main` once `just check` is green.
- Conventional Commits with the task id as scope: `feat(dv0002): parse inline fields`.
- Mark §T status `.` → `~` → `x` as work progresses. Log bugs in §B via `/spec bug:`.
- `just check` is the definition of green (go vet, go test, headless Neovim smoke test). Run it before calling anything done.
- New DQL behaviour needs a golden test against `testdata/vault/` whose expected output comes from real Obsidian Dataview (§V.1).
