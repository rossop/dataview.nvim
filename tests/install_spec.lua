-- Installer against a fake release folder served over file:// (curl handles
-- it), so no network is needed.
local install = require("dataview.install")
local root = vim.fn.getcwd()

local function fake_release(tamper)
  local dir = vim.fn.tempname()
  vim.fn.mkdir(dir, "p")
  local asset = assert(install.asset())
  local f = io.open(root .. "/bin/dataview", "rb")
  local data = f:read("*a")
  f:close()
  local out = io.open(dir .. "/" .. asset, "wb")
  out:write(tamper and (data .. "x") or data)
  out:close()
  vim.fn.writefile({
    vim.fn.sha256(data) .. "  " .. asset,
    ("0"):rep(64) .. "  dataview_plan9_mips",
  }, dir .. "/checksums.txt")
  return "file://" .. dir .. "/"
end

local function run_install()
  local result
  install.install(function(err, path)
    result = { err = err, path = path }
  end, { quiet = true })
  assert(vim.wait(10000, function()
    return result ~= nil
  end, 10), "install timed out")
  return result
end

local orig_url, orig_dir = install.base_url, install.dir

local function with_release(url, fn)
  local data = vim.fn.tempname()
  install.base_url = function()
    return url
  end
  install.dir = function()
    return data .. "/dataview/bin"
  end
  local ok, err = pcall(fn)
  install.base_url, install.dir = orig_url, orig_dir
  assert(ok, err)
end

return {
  ["parses sha256sum output"] = function()
    eq({ a = "abcd", b = "ef01" }, install.parse_checksums("ABCD  a\nef01 *b\n\ngarbage"))
  end,

  ["asset name matches this machine"] = function()
    local asset = assert(install.asset())
    assert(asset:match("^dataview_%l+_%w+"), asset)
  end,

  ["verified download installs an executable binary"] = function()
    with_release(fake_release(false), function()
      local r = run_install()
      eq(nil, r.err)
      eq(1, vim.fn.executable(r.path))
      eq("dev", install.binary_version(r.path))
    end)
  end,

  ["checksum mismatch refuses to install (V15)"] = function()
    with_release(fake_release(true), function()
      local r = run_install()
      assert(r.err and r.err:find("checksum mismatch"), vim.inspect(r))
      eq(0, vim.fn.filereadable(install.path()))
    end)
  end,

  ["missing release is a clear error"] = function()
    with_release("file:///nonexistent/", function()
      local r = run_install()
      assert(r.err and r.err:find("download failed"), vim.inspect(r))
    end)
  end,
}
