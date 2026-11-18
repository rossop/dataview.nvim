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
}

M.options = vim.deepcopy(M.defaults)

function M.setup(opts)
  M.options = vim.tbl_deep_extend("force", vim.deepcopy(M.defaults), opts or {})
end

return M
