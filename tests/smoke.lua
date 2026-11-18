-- Headless smoke test: the plugin must load with zero dependencies.
vim.opt.rtp:prepend(vim.fn.getcwd())
local ok, err = pcall(require, "dataview")
if not ok then
  io.stderr:write("require('dataview') failed: " .. tostring(err) .. "\n")
  os.exit(1)
end
print("smoke: ok")
