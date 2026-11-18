-- End to end: real binary (built by `just build`), temp copy of the fixture
-- vault (§V.2: tests never write to testdata/vault).
local dv = require("dataview")
local inline = require("dataview.render.inline")
local root = vim.fn.getcwd()

vim.env.XDG_CACHE_HOME = vim.fn.tempname()

local function copy_vault(extra)
  local dir = vim.fn.tempname()
  vim.fn.mkdir(dir, "p")
  vim.system({ "cp", "-R", root .. "/testdata/vault/.", dir }):wait()
  for name, lines in pairs(extra or {}) do
    vim.fn.mkdir(vim.fn.fnamemodify(dir .. "/" .. name, ":h"), "p")
    vim.fn.writefile(lines, dir .. "/" .. name)
  end
  return vim.fn.resolve(dir)
end

local function virt_text(buf)
  local out = {}
  for _, m in ipairs(vim.api.nvim_buf_get_extmarks(buf, inline.ns, 0, -1, { details = true })) do
    for _, chunks in ipairs(m[4].virt_lines or {}) do
      local s = ""
      for _, c in ipairs(chunks) do
        s = s .. c[1]
      end
      out[#out + 1] = s
    end
  end
  return table.concat(out, "\n")
end

local function wait_for(buf, needle)
  local ok = vim.wait(5000, function()
    return virt_text(buf):find(needle, 1, true) ~= nil
  end, 10)
  if not ok then
    error(("timed out waiting for %q; got:\n%s"):format(needle, virt_text(buf)), 2)
  end
end

local dashboard = {
  "# Dashboard",
  "```dataview",
  'TABLE status FROM "Projects" SORT file.name',
  "```",
  "",
  "```dataview",
  "TASK WHERE !completed",
  "```",
}

return {
  ["opening a note renders its blocks"] = function()
    local vault = copy_vault({ ["Dashboard.md"] = dashboard })
    vim.cmd.edit(vault .. "/Dashboard.md")
    local buf = vim.api.nvim_get_current_buf()
    wait_for(buf, "active")
    wait_for(buf, "Draft outline")
    assert(dv.is_shown(buf))
  end,

  ["saving a note refreshes visible results"] = function()
    local vault = copy_vault({ ["Dashboard.md"] = dashboard })
    vim.cmd.edit(vault .. "/Dashboard.md")
    local dash = vim.api.nvim_get_current_buf()
    wait_for(dash, "Beta")

    vim.cmd.edit(vault .. "/Projects/Gamma.md")
    vim.api.nvim_buf_set_lines(0, 0, -1, false, { "---", "status: brand-new", "---" })
    vim.cmd("silent write")
    wait_for(dash, "brand-new")
  end,

  ["toggle clears and re-runs"] = function()
    local vault = copy_vault({ ["Dashboard.md"] = dashboard })
    vim.cmd.edit(vault .. "/Dashboard.md")
    local buf = vim.api.nvim_get_current_buf()
    wait_for(buf, "Alpha")
    vim.cmd("Dataview toggle")
    eq("", virt_text(buf))
    vim.cmd("Dataview toggle")
    wait_for(buf, "Alpha")
  end,

  ["query errors show inline with position"] = function()
    local vault = copy_vault({ ["Bad.md"] = { "```dataview", "LIST WHERE (a", "```" } })
    vim.cmd.edit(vault .. "/Bad.md")
    wait_for(vim.api.nvim_get_current_buf(), 'dataview: 1:14: expected ")"')
  end,

  ["protocol mismatch gives a clear error (V11)"] = function()
    if vim.fn.has("win32") == 1 then
      return
    end
    local fake = vim.fn.tempname()
    vim.fn.writefile({
      "#!/bin/sh",
      "read line",
      [[echo '{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":99,"version":"old","notes":0}}']],
      "cat >/dev/null",
    }, fake)
    vim.fn.setfperm(fake, "rwxr-xr-x")
    dv.setup({ binary = fake })
    local vault = copy_vault({ ["Q.md"] = { "```dataview", "LIST", "```" } })
    vim.cmd.edit(vault .. "/Q.md")
    local ok, err = pcall(wait_for, vim.api.nvim_get_current_buf(), "speaks protocol 99")
    dv.setup({})
    assert(ok, err)
  end,

  ["missing binary is reported, not thrown"] = function()
    dv.setup({ binary = "/nonexistent/dataview" })
    local vault = copy_vault({ ["Q.md"] = { "```dataview", "LIST", "```" } })
    vim.cmd.edit(vault .. "/Q.md")
    local ok, err = pcall(wait_for, vim.api.nvim_get_current_buf(), "cannot start")
    dv.setup({})
    assert(ok, err)
  end,
}
