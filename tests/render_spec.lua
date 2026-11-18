local inline = require("dataview.render.inline")
local opts = { max_rows = 2, max_cell_width = 8 }

local function text(lines)
  local out = {}
  for _, chunks in ipairs(lines) do
    local s = ""
    for _, c in ipairs(chunks) do
      s = s .. c[1]
    end
    out[#out + 1] = vim.trim(s)
  end
  return out
end

return {
  ["table aligns, truncates and limits rows"] = function()
    local res = {
      kind = "table",
      columns = { "File", "status" },
      rows = {
        { path = "a.md", cells = { "Alpha", "active" } },
        { path = "b.md", cells = { "Beta", "a very long status" } },
        { path = "c.md", cells = { "Gamma", "x" } },
      },
    }
    eq({
      "File   status",
      "───────────────",
      "Alpha  active",
      "Beta   a very …",
      "… 1 more · 3 results · 5 ms",
    }, text(inline.format(res, nil, opts, 5)))
  end,
  ["list with and without values"] = function()
    local res = { kind = "list", rows = { { cells = { "Alpha", "Due 1" } }, { cells = { "Beta" } } } }
    eq({ "• Alpha: Due 1", "• Beta", "2 results" }, text(inline.format(res, nil, opts)))
  end,
  ["tasks grouped by file"] = function()
    local res = {
      kind = "task",
      rows = {
        { path = "P/Alpha.md", cells = { "one" }, task = { completed = false } },
        { path = "P/Alpha.md", cells = { "two" }, task = { completed = true } },
      },
    }
    eq({ "Alpha", "☐ one", "☑ two", "2 results" }, text(inline.format(res, nil, opts)))
  end,
  ["errors and empty results"] = function()
    eq({ "dataview: 1:6: bad" }, text(inline.format(nil, "1:6: bad", opts)))
    eq({ "No results" }, text(inline.format({ kind = "list", rows = {} }, nil, opts)))
  end,
}
