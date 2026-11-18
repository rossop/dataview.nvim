-- Show query results in whatever picker the user has. None is required
-- (§V.12): each adapter is loaded with pcall and vim.ui.select always works.
local M = {}

M.order = { "telescope", "fzf", "snacks", "select" }

local function available(name)
  if name == "telescope" then
    return (pcall(require, "telescope"))
  elseif name == "fzf" then
    return (pcall(require, "fzf-lua"))
  elseif name == "snacks" then
    local ok, snacks = pcall(require, "snacks")
    return ok and snacks.picker ~= nil
  end
  return name == "select"
end

--- One entry per result row: display text plus where it lives.
function M.items(res, vault)
  local items = {}
  for _, row in ipairs(res.rows or {}) do
    local cells = vim.tbl_map(function(c)
      return (tostring(c):gsub("[\r\n]+", " "))
    end, row.cells or {})
    local text = table.concat(cells, " │ ")
    if res.kind == "task" then
      local done = row.task and row.task.completed
      text = (done and "☑ " or "☐ ") .. text .. "  · " .. vim.fn.fnamemodify(row.path, ":t:r")
    end
    items[#items + 1] = {
      text = text,
      path = row.path,
      file = vault .. "/" .. row.path,
      lnum = row.task and (row.task.line + 1) or 1,
      row = row,
    }
  end
  return items
end

function M.pick(res, ctx)
  local name = require("dataview.config").options.picker
  if name == "auto" then
    for _, candidate in ipairs(M.order) do
      if available(candidate) then
        name = candidate
        break
      end
    end
  elseif not available(name) then
    vim.notify(("dataview: picker %q not available, using vim.ui.select"):format(name), vim.log.levels.WARN)
    name = "select"
  end
  local items = M.items(res, ctx.vault)
  if #items == 0 then
    return vim.notify("dataview: no results", vim.log.levels.INFO)
  end
  return require("dataview.pickers." .. name)(items, ctx)
end

return M
