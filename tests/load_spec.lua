-- §V.12: the plugin loads and registers its command with zero dependencies.
return {
  ["requires without setup"] = function()
    assert(require("dataview"))
    eq(2, vim.fn.exists(":Dataview"))
  end,
  ["setup merges options"] = function()
    require("dataview").setup({ max_rows = 3 })
    eq(3, require("dataview.config").options.max_rows)
    eq(true, require("dataview.config").options.auto)
    require("dataview").setup({})
  end,
  ["checkhealth reports binary and vault"] = function()
    local vault = vim.fn.tempname()
    vim.fn.mkdir(vault .. "/.obsidian", "p")
    vim.cmd.edit(vault .. "/N.md")
    require("dataview").setup({ binary = vim.fn.getcwd() .. "/bin/dataview" })
    vim.cmd("silent checkhealth dataview")
    local text = table.concat(vim.api.nvim_buf_get_lines(0, 0, -1, false), "\n")
    vim.cmd("bwipeout!")
    require("dataview").setup({})
    assert(text:find("binary .*%(vdev%)"), text)
    assert(text:find("vault: .*nearest %.obsidian"), text)
    assert(not text:find("ERROR"), text)
  end,
}
