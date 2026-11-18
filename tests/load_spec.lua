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
}
