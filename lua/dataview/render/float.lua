-- Results in a scrollable floating window. <CR> opens the row's note,
-- q / <Esc> closes.
local inline = require("dataview.render.inline")

local M = {}

function M.show(res, err, ctx)
  local opts = require("dataview.config").options
  -- In a float there is room for every row.
  local fopts = vim.tbl_extend("force", opts, { max_rows = math.huge })
  local chunks, rowmap = inline.build(res, err, fopts, ctx.ms)

  local text, marks = {}, {}
  for i, line in ipairs(chunks) do
    local s, col = "", 0
    for _, c in ipairs(line) do
      s = s .. c[1]
      marks[#marks + 1] = { i - 1, col, col + #c[1], c[2] }
      col = col + #c[1]
    end
    text[i] = s
  end

  local buf = vim.api.nvim_create_buf(false, true)
  vim.api.nvim_buf_set_lines(buf, 0, -1, false, text)
  for _, m in ipairs(marks) do
    vim.api.nvim_buf_set_extmark(buf, inline.ns, m[1], m[2], { end_col = m[3], hl_group = m[4] })
  end
  vim.bo[buf].modifiable = false
  vim.bo[buf].bufhidden = "wipe"
  vim.bo[buf].filetype = "dataview"

  local width = 0
  for _, l in ipairs(text) do
    width = math.max(width, vim.fn.strdisplaywidth(l))
  end
  local fo = opts.float
  width = math.min(math.max(width + 2, 30), math.floor(vim.o.columns * fo.width))
  local height = math.min(#text, math.floor(vim.o.lines * fo.height))
  local win = vim.api.nvim_open_win(buf, true, {
    relative = "editor",
    width = width,
    height = math.max(height, 1),
    row = math.floor((vim.o.lines - height) / 2) - 1,
    col = math.floor((vim.o.columns - width) / 2),
    border = fo.border,
    title = " " .. (ctx.title or "dataview") .. " ",
    title_pos = "center",
    style = "minimal",
  })
  vim.wo[win].cursorline = true
  vim.wo[win].wrap = false

  local function close()
    if vim.api.nvim_win_is_valid(win) then
      vim.api.nvim_win_close(win, true)
    end
  end
  local function open_row()
    local row = rowmap[vim.api.nvim_win_get_cursor(win)[1]]
    if not row then
      return
    end
    close()
    ctx.open(row)
  end
  for _, lhs in ipairs({ "q", "<Esc>" }) do
    vim.keymap.set("n", lhs, close, { buffer = buf, nowait = true })
  end
  vim.keymap.set("n", "<CR>", open_row, { buffer = buf, nowait = true })
  return buf, win
end

return M
