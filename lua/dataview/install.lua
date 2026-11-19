-- Download the prebuilt binary for this OS/arch from the GitHub release that
-- matches the plugin version, verify its SHA256 (§V.15), and put it in
-- stdpath("data")/dataview/bin. Async: Neovim stays responsive (§V.8).
local M = {}

M.repo = "rossop/dataview.nvim"

local is_windows = vim.fn.has("win32") == 1

function M.dir()
  return vim.fn.stdpath("data") .. "/dataview/bin"
end

function M.path()
  return M.dir() .. "/" .. (is_windows and "dataview.exe" or "dataview")
end

--- Release asset name, e.g. dataview_darwin_arm64 or dataview_windows_amd64.exe.
function M.asset()
  local uname = vim.uv.os_uname()
  local os_name = ({ Darwin = "darwin", Linux = "linux", Windows_NT = "windows" })[uname.sysname]
  local machine = uname.machine:lower()
  local arch = ({ x86_64 = "amd64", amd64 = "amd64", arm64 = "arm64", aarch64 = "arm64" })[machine]
  if not os_name or not arch then
    return nil, ("no prebuilt binary for %s/%s; build it with `go build ./cmd/dataview`"):format(uname.sysname, machine)
  end
  return ("dataview_%s_%s%s"):format(os_name, arch, os_name == "windows" and ".exe" or "")
end

--- Folder URL holding the release assets. Tests point this at file://.
function M.base_url(version)
  return ("https://github.com/%s/releases/download/v%s/"):format(M.repo, version)
end

local function fetch(url, cb)
  vim.system({ "curl", "-fsSL", "--retry", "2", url }, { text = false }, function(obj)
    vim.schedule(function()
      if obj.code ~= 0 then
        return cb(("download failed (%s): %s"):format(url, vim.trim(obj.stderr or "")))
      end
      cb(nil, obj.stdout)
    end)
  end)
end

--- Parse a sha256sum-style checksums file into { [name] = hex }.
function M.parse_checksums(text)
  local sums = {}
  for line in text:gmatch("[^\r\n]+") do
    local hex, name = line:match("^(%x+)%s+%*?(%S+)$")
    if hex then
      sums[name] = hex:lower()
    end
  end
  return sums
end

local running = nil -- callbacks waiting on an install in progress

--- Install (or reinstall) the binary. cb(err, path) runs on the main loop.
--- Concurrent calls share one download.
function M.install(cb, opts)
  cb = cb or function() end
  opts = opts or {}
  if running then
    table.insert(running, cb)
    return
  end
  running = { cb }
  local function done(err, path)
    local waiting = running
    running = nil
    for _, f in ipairs(waiting) do
      f(err, path)
    end
  end

  if vim.fn.executable("curl") == 0 then
    return done("curl is required to download the dataview binary")
  end
  local asset, err = M.asset()
  if not asset then
    return done(err)
  end
  local version = opts.version or require("dataview.version")
  local url = M.base_url(version)
  if not opts.quiet then
    vim.notify(("dataview: downloading %s v%s…"):format(asset, version), vim.log.levels.INFO)
  end

  fetch(url .. "checksums.txt", function(cerr, sums_text)
    if cerr then
      return done(cerr)
    end
    local want = M.parse_checksums(sums_text)[asset]
    if not want then
      return done("checksums.txt has no entry for " .. asset)
    end
    fetch(url .. asset, function(ferr, data)
      if ferr then
        return done(ferr)
      end
      local got = vim.fn.sha256(data)
      if got ~= want then
        return done(("checksum mismatch for %s: want %s, got %s; not installing"):format(asset, want, got))
      end
      vim.fn.mkdir(M.dir(), "p")
      -- Write beside the target then rename, so a running copy is never
      -- replaced by a half-written file.
      local tmp = M.path() .. ".tmp"
      local f, oerr = io.open(tmp, "wb")
      if not f then
        return done("cannot write " .. tmp .. ": " .. tostring(oerr))
      end
      f:write(data)
      f:close()
      if not is_windows then
        vim.uv.fs_chmod(tmp, tonumber("755", 8))
      end
      local ok, rerr = vim.uv.fs_rename(tmp, M.path())
      if not ok then
        return done("cannot install binary: " .. tostring(rerr))
      end
      if not opts.quiet then
        vim.notify("dataview: installed " .. M.path(), vim.log.levels.INFO)
      end
      done(nil, M.path())
    end)
  end)
end

--- Installed binary version, or nil. Blocking, so only for :checkhealth.
function M.binary_version(bin)
  local ok, obj = pcall(function()
    return vim.system({ bin, "version" }, { text = true }):wait(3000)
  end)
  if ok and obj.code == 0 then
    return vim.trim(obj.stdout)
  end
end

return M
