# dataview.nvim

Run [Obsidian Dataview](https://blacksmithgu.github.io/obsidian-dataview/) queries in Neovim. Open a note with a ` ```dataview ` block and the results appear under it, without Obsidian running.

- **No setup.** Add the plugin; a small, checksum-verified binary downloads on first use. No Go toolchain, no other plugins.
- **Fast.** Queries answer in milliseconds; a 10,000-note vault indexes in about 0.3 s, and later sessions reuse an on-disk cache.
- **Light.** One background process per vault while Neovim runs (about 15 MB of index for 10k notes), gone when you quit.
- **Read-only.** Your vault is never written to.

## Install

[lazy.nvim](https://github.com/folke/lazy.nvim):

```lua
{ "rossop/dataview.nvim", ft = "markdown" }
```

That's all. The vault is the nearest folder containing `.obsidian/`. Run `:checkhealth dataview` if something looks wrong.

Other plugin managers work the same way: add the repo; the binary installs itself on first query (or run `:Dataview install`).

<details>
<summary>Build the binary yourself instead</summary>

With Go installed, build into the plugin's `bin/` folder, which is checked first:

```lua
{ "rossop/dataview.nvim", ft = "markdown", build = "go build -o bin/dataview ./cmd/dataview" }
```

Or put `dataview` on your `$PATH`, or set `binary = "/path/to/dataview"`.
</details>

## Use

````markdown
```dataview
TABLE status, due FROM "Projects" WHERE status != "done" SORT due
```
````

| Command | |
|---|---|
| `:Dataview` / `:Dataview run` | Run every block in the buffer, results inline |
| `:Dataview toggle` | Show or hide inline results |
| `:Dataview float [query]` | Block under the cursor (or an ad-hoc query) in a float; `<CR>` opens the note |
| `:Dataview pick [query]` | Same, in Telescope, fzf-lua, snacks or `vim.ui.select`, whichever you have |
| `:Dataview refresh` | Rescan the whole vault (after bulk changes outside Neovim) |
| `:Dataview install` | Download or update the binary |
| `:Dataview log` | Show the binary's log |

Results render automatically when a note with blocks opens, and refresh when you save any note in the vault. There are no default keymaps; map the `<Plug>` ones you want:

```lua
vim.keymap.set("n", "<leader>dr", "<Plug>(dataview-run)")
vim.keymap.set("n", "<leader>dt", "<Plug>(dataview-toggle)")
vim.keymap.set("n", "<leader>df", "<Plug>(dataview-float)")
vim.keymap.set("n", "<leader>dp", "<Plug>(dataview-pick)")
```

## Options

Defaults; pass only what you change to `require("dataview").setup({...})` (or `opts = {...}` in lazy.nvim).

```lua
{
  vault = nil,            -- nil = nearest folder with .obsidian/
  binary = nil,           -- nil = plugin bin/, auto-installed copy, then $PATH
  auto_install = true,    -- download the binary on first use
  auto = true,            -- render blocks when a note opens
  refresh_on_save = true, -- re-run visible results after a save
  max_rows = 50,          -- inline rows before "… N more"
  max_cell_width = 40,
  picker = "auto",        -- "telescope" | "fzf" | "snacks" | "select"
  float = { width = 0.8, height = 0.6, border = "rounded" },
}
```

Highlight groups (all linked to standard groups by default): `DataviewHeader`, `DataviewFile`, `DataviewBorder`, `DataviewMuted`, `DataviewError`, `DataviewTaskOpen`, `DataviewTaskDone`.

## What's supported

DQL `TABLE` (with `WITHOUT ID`, `AS`), `LIST` (with an expression), `TASK`; `FROM` with `#tags` (nested), `"folders"`, `[[links]]`, `outgoing([[links]])`, `and`/`or`/`-`; `WHERE`, `SORT`, `LIMIT` in any order. Frontmatter, inline `key:: value` / `[key:: value]` fields, `file.*` metadata, dates and durations (`date(today)`, `dur(1 week)`), link fields (`up.status`), and common functions (`contains`, `length`, `dateformat`, `default`, `round`, `join`, …).

Not yet: `GROUP BY`, `FLATTEN`, `CALENDAR`, DataviewJS. Those queries show a clear "not supported yet" message.

## Command line

The binary also works on its own:

```sh
dataview query --vault ~/Notes --dql 'LIST FROM #project'   # JSON on stdout
```

## Development

`just check` runs `go vet`, the Go tests and the headless Neovim specs. `SPEC.md` holds the goals, invariants and task list; `AGENTS.md` the working rules. Releases: bump `lua/dataview/version.lua`, `just release <version>`, push the tag.
