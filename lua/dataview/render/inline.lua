-- Render results as virtual lines under each block's closing fence. Pure
-- layout: every cell already arrives as display text from the binary (§V.9).
local M = {}

M.ns = vim.api.nvim_create_namespace("dataview")

local function set_hl()
  local links = {
    DataviewHeader = "Title",
    DataviewBorder = "FloatBorder",
    DataviewMuted = "Comment",
    DataviewError = "DiagnosticError",
    DataviewTaskDone = "Comment",
    DataviewTaskOpen = "Normal",
    DataviewFile = "Directory",
  }
  for group, link in pairs(links) do
    vim.api.nvim_set_hl(0, group, { link = link, default = true })
  end
end
set_hl()
vim.api.nvim_create_autocmd("ColorScheme", { callback = set_hl })

local function clean(s, max)
  s = tostring(s or ""):gsub("[\r\n]+", " ")
  if max and vim.fn.strdisplaywidth(s) > max then
    s = vim.fn.strcharpart(s, 0, max - 1) .. "…"
  end
  return s
end

local function pad(s, width)
  return s .. string.rep(" ", width - vim.fn.strdisplaywidth(s))
end

local function footer(lines, shown, total, ms)
  local text = ("%d result%s"):format(total, total == 1 and "" or "s")
  if shown < total then
    text = ("… %d more · %s"):format(total - shown, text)
  end
  if ms then
    text = text .. (" · %d ms"):format(ms)
  end
  lines[#lines + 1] = { { text, "DataviewMuted" } }
end

-- rowmap[i] = result row shown on virt line i (nil for headers/footers).
local function format_table(res, opts, lines, rowmap)
  local cols = res.columns or {}
  local rows = res.rows or {}
  local n = math.min(#rows, opts.max_rows)
  local widths = {}
  for i, c in ipairs(cols) do
    widths[i] = vim.fn.strdisplaywidth(clean(c, opts.max_cell_width))
  end
  for r = 1, n do
    for i, cell in ipairs(rows[r].cells) do
      widths[i] = math.max(widths[i] or 0, vim.fn.strdisplaywidth(clean(cell, opts.max_cell_width)))
    end
  end
  local header, rule = {}, {}
  for i, c in ipairs(cols) do
    header[#header + 1] = { pad(clean(c, opts.max_cell_width), widths[i]), "DataviewHeader" }
    header[#header + 1] = { "  ", "DataviewBorder" }
    rule[#rule + 1] = string.rep("─", widths[i])
  end
  lines[#lines + 1] = header
  lines[#lines + 1] = { { table.concat(rule, "──"), "DataviewBorder" } }
  for r = 1, n do
    local line = {}
    for i, cell in ipairs(rows[r].cells) do
      local hl = (i == 1 and cols[1] == "File") and "DataviewFile" or "Normal"
      line[#line + 1] = { pad(clean(cell, opts.max_cell_width), widths[i] or 0), hl }
      line[#line + 1] = { "  ", "Normal" }
    end
    lines[#lines + 1] = line
    rowmap[#lines] = rows[r]
  end
  return n
end

local function format_list(res, opts, lines, rowmap)
  local rows = res.rows or {}
  local n = math.min(#rows, opts.max_rows)
  for r = 1, n do
    local cells = rows[r].cells
    local line = { { "• ", "DataviewMuted" } }
    if #cells == 2 then
      line[#line + 1] = { clean(cells[1]), "DataviewFile" }
      line[#line + 1] = { ": " .. clean(cells[2]), "Normal" }
    else
      line[#line + 1] = { clean(cells[1]), "DataviewFile" }
    end
    lines[#lines + 1] = line
    rowmap[#lines] = rows[r]
  end
  return n
end

-- Tasks are grouped under their file, as in Obsidian.
local function format_tasks(res, opts, lines, rowmap)
  local rows = res.rows or {}
  local n = math.min(#rows, opts.max_rows)
  local last_path
  for r = 1, n do
    local row = rows[r]
    if row.path ~= last_path then
      last_path = row.path
      lines[#lines + 1] = { { vim.fn.fnamemodify(row.path, ":t:r"), "DataviewFile" } }
    end
    local done = row.task and row.task.completed
    lines[#lines + 1] = {
      { done and "  ☑ " or "  ☐ ", "DataviewMuted" },
      { clean(row.cells[1]), done and "DataviewTaskDone" or "DataviewTaskOpen" },
    }
    rowmap[#lines] = row
  end
  return n
end

--- Turn a query result (or error) into virt_lines chunks plus a map from
--- line number to the result row it shows.
function M.build(res, err, opts, ms)
  local lines, rowmap = {}, {}
  if err then
    for line in tostring(err):gmatch("[^\n]+") do
      lines[#lines + 1] = { { "dataview: " .. line, "DataviewError" } }
    end
    return lines, rowmap
  end
  if not res.rows or #res.rows == 0 then
    lines[#lines + 1] = { { "No results", "DataviewMuted" } }
    return lines, rowmap
  end
  local shown
  if res.kind == "table" then
    shown = format_table(res, opts, lines, rowmap)
  elseif res.kind == "task" then
    shown = format_tasks(res, opts, lines, rowmap)
  else
    shown = format_list(res, opts, lines, rowmap)
  end
  footer(lines, shown, #res.rows, ms)
  return lines, rowmap
end

--- Virt lines only. Exposed for tests.
function M.format(res, err, opts, ms)
  return (M.build(res, err, opts, ms))
end

local function key(buf, block)
  return buf .. ":" .. block.start_row
end

local marks = {} -- key -> extmark id

function M.show(buf, block, lines)
  if not vim.api.nvim_buf_is_valid(buf) then
    return
  end
  local k = key(buf, block)
  local row = math.min(block.end_row, vim.api.nvim_buf_line_count(buf) - 1)
  local virt = { { { "", "Normal" } } }
  vim.list_extend(virt, lines)
  marks[k] = vim.api.nvim_buf_set_extmark(buf, M.ns, row, 0, {
    id = marks[k],
    virt_lines = virt,
    undo_restore = false,
    invalidate = true,
  })
end

function M.clear(buf)
  if vim.api.nvim_buf_is_valid(buf) then
    vim.api.nvim_buf_clear_namespace(buf, M.ns, 0, -1)
  end
  local prefix = buf .. ":"
  for k in pairs(marks) do
    if k:sub(1, #prefix) == prefix then
      marks[k] = nil
    end
  end
end

return M
