-- Defaults and user options. Everything works without calling setup().
local M = {}

M.defaults = {
  -- Vault root. nil = nearest parent folder containing .obsidian/.
  vault = nil,
  -- Path to the dataview binary. nil = auto (see dataview.binary()).
  binary = nil,
  -- Render results automatically when a note with dataview blocks is shown.
  auto = true,
  -- Re-run visible results after any note in the vault is written.
  refresh_on_save = true,
  -- Rows shown inline before "… N more".
  max_rows = 50,
  -- Cells wider than this are truncated with "…".
  max_cell_width = 40,
  -- :Dataview pick backend: "auto" (telescope → fzf-lua → snacks →
  -- vim.ui.select) or one of those names.
  picker = "auto",
  -- :Dataview float window size as a fraction of the editor, and border.
  float = { width = 0.8, height = 0.6, border = "rounded" },
}

M.options = vim.deepcopy(M.defaults)

function M.setup(opts)
  M.options = vim.tbl_deep_extend("force", vim.deepcopy(M.defaults), opts or {})
end

return M
