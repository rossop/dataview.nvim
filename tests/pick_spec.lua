-- Float and picker, driven through the real binary on a temp vault copy.
local dv = require("dataview")
local root = vim.fn.getcwd()

local function vault_with_note()
  local dir = vim.fn.tempname()
  vim.fn.mkdir(dir, "p")
  vim.system({ "cp", "-R", root .. "/testdata/vault/.", dir }):wait()
  vim.fn.writefile({ "```dataview", "TASK WHERE !completed SORT file.name", "```" }, dir .. "/T.md")
  return vim.fn.resolve(dir)
end

local function wait(cond, what)
  assert(vim.wait(5000, cond, 10), "timed out: " .. what)
end

return {
  ["items carry file, line and display text"] = function()
    local items = require("dataview.pickers").items({
      kind = "task",
      rows = { { path = "P/A.md", cells = { "do\nit" }, task = { line = 4, completed = false } } },
    }, "/v")
    eq("☐ do it  · A", items[1].text)
    eq("/v/P/A.md", items[1].file)
    eq(5, items[1].lnum)
  end,

  ["vim.ui.select fallback opens the task line"] = function()
    local vault = vault_with_note()
    vim.cmd.edit(vault .. "/T.md")
    require("dataview").setup({ picker = "select", auto = false })
    local offered
    local orig = vim.ui.select
    vim.ui.select = function(items, opts, cb)
      offered = vim.tbl_map(opts.format_item, items)
      cb(items[2]) -- "Draft outline" in Projects/Alpha.md, line 12
    end
    dv.pick()
    wait(function()
      return offered ~= nil
    end, "picker")
    vim.ui.select = orig
    require("dataview").setup({})
    eq("☐ Draft outline  · Alpha", offered[2])
    eq(vault .. "/Projects/Alpha.md", vim.api.nvim_buf_get_name(0))
    eq(13, vim.api.nvim_win_get_cursor(0)[1])
  end,

  ["ad-hoc query in a float, <CR> opens the note"] = function()
    local vault = vault_with_note()
    vim.cmd.edit(vault .. "/T.md")
    vim.cmd('Dataview float TABLE status FROM "Projects" SORT file.name')
    wait(function()
      return vim.bo.filetype == "dataview"
    end, "float")
    local lines = vim.api.nvim_buf_get_lines(0, 0, -1, false)
    assert(table.concat(lines, "\n"):find("Beta%s+done"), vim.inspect(lines))
    vim.api.nvim_win_set_cursor(0, { 4, 0 }) -- header, rule, Alpha, Beta
    vim.api.nvim_feedkeys(vim.api.nvim_replace_termcodes("<CR>", true, false, true), "x", false)
    eq(vault .. "/Projects/Beta.md", vim.api.nvim_buf_get_name(0))
  end,
}
