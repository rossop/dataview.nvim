local blocks = require("dataview.blocks")

local function buf_with(lines)
  local b = vim.api.nvim_create_buf(false, true)
  vim.api.nvim_buf_set_lines(b, 0, -1, false, lines)
  vim.bo[b].filetype = "markdown"
  return b
end

local doc = {
  "# Note", --0
  "```dataview", --1
  "LIST", --2
  "FROM #x", --3
  "```", --4
  "```js", --5
  "```dataview inside js is not a block", --6
  "```", --7
  "- item", --8
  "  ~~~~dataview", --9
  "  TABLE a", --10
  "  ~~~~", --11
  "```dataview", --12 unclosed
  "LIST",
}

return {
  ["line scanner finds closed dataview fences"] = function()
    local got = blocks.scan_lines(doc)
    eq(2, #got)
    eq({ start_row = 1, end_row = 4, query = "LIST\nFROM #x" }, got[1])
    eq({ start_row = 9, end_row = 11, query = "TABLE a" }, got[2])
  end,
  ["find works on a buffer"] = function()
    local got = blocks.find(buf_with(doc))
    eq(1, got[1].start_row)
    eq(4, got[1].end_row)
    eq("LIST\nFROM #x", got[1].query)
  end,
  ["has_blocks is a cheap pre-check"] = function()
    eq(true, blocks.has_blocks(buf_with({ "x", "```Dataview", "LIST", "```" })))
    eq(false, blocks.has_blocks(buf_with({ "```js", "dataview", "```" })))
  end,
}
