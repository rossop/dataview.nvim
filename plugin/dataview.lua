-- Registers :Dataview, <Plug> mappings and autocmds. Modules load on first
-- use, so startup cost is a few function definitions. No default keymaps.
if vim.g.loaded_dataview then
  return
end
vim.g.loaded_dataview = true

if vim.fn.has("nvim-0.10") == 0 then
  vim.notify("dataview.nvim needs Neovim 0.10 or newer", vim.log.levels.ERROR)
  return
end

local subcommands = {
  run = function()
    if require("dataview").run() == 0 then
      require("dataview")._notify("no ```dataview blocks in this buffer", vim.log.levels.INFO)
    end
  end,
  toggle = function()
    require("dataview").toggle()
  end,
  clear = function()
    require("dataview").clear()
  end,
  refresh = function()
    require("dataview").refresh()
  end,
  log = function()
    require("dataview").log()
  end,
}

vim.api.nvim_create_user_command("Dataview", function(args)
  local sub = args.fargs[1] or "run"
  local fn = subcommands[sub]
  if not fn then
    return vim.notify("dataview: unknown subcommand " .. sub, vim.log.levels.ERROR)
  end
  fn()
end, {
  nargs = "?",
  desc = "Run Obsidian Dataview queries",
  complete = function(lead)
    return vim.tbl_filter(function(s)
      return vim.startswith(s, lead)
    end, vim.tbl_keys(subcommands))
  end,
})

vim.keymap.set("n", "<Plug>(dataview-run)", subcommands.run, { desc = "Dataview: run blocks" })
vim.keymap.set("n", "<Plug>(dataview-toggle)", subcommands.toggle, { desc = "Dataview: toggle results" })
vim.keymap.set("n", "<Plug>(dataview-refresh)", subcommands.refresh, { desc = "Dataview: rescan vault" })

local group = vim.api.nvim_create_augroup("dataview", { clear = true })
vim.api.nvim_create_autocmd("BufWinEnter", {
  group = group,
  pattern = "*.md",
  callback = function(ev)
    require("dataview").on_enter(ev.buf)
  end,
})
vim.api.nvim_create_autocmd("BufWritePost", {
  group = group,
  pattern = "*.md",
  callback = function(ev)
    if package.loaded["dataview.process"] then -- nothing to refresh before first use
      require("dataview").on_write(ev.buf)
    end
  end,
})
vim.api.nvim_create_autocmd("VimLeavePre", {
  group = group,
  callback = function()
    if package.loaded["dataview.process"] then
      require("dataview.process").stop_all()
    end
  end,
})
