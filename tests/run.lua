-- Minimal headless test runner: nvim --headless -u NONE -l tests/run.lua
-- Each tests/*_spec.lua returns { ["name"] = function() ... end }.
-- No test framework dependency, so `just check` needs only Neovim.
local root = vim.fn.getcwd()
vim.opt.rtp:prepend(root)
vim.cmd("runtime plugin/dataview.lua")

_G.eq = function(want, got, msg)
  if not vim.deep_equal(want, got) then
    error((msg or "not equal") .. "\nwant: " .. vim.inspect(want) .. "\n got: " .. vim.inspect(got), 2)
  end
end

local filter = os.getenv("TEST")
local failed, passed = 0, 0
local specs = vim.fn.glob(root .. "/tests/*_spec.lua", false, true)
table.sort(specs)
for _, file in ipairs(specs) do
  local cases = dofile(file)
  local names = vim.tbl_keys(cases)
  table.sort(names)
  for _, name in ipairs(names) do
    local full = vim.fn.fnamemodify(file, ":t:r") .. ": " .. name
    if not filter or full:find(filter, 1, true) then
      local ok, err = xpcall(cases[name], debug.traceback)
      if ok then
        passed = passed + 1
        io.stdout:write("ok   " .. full .. "\n")
      else
        failed = failed + 1
        io.stdout:write("FAIL " .. full .. "\n" .. tostring(err) .. "\n")
      end
    end
  end
end
io.stdout:write(("%d passed, %d failed\n"):format(passed, failed))
os.exit(failed == 0 and 0 or 1)
