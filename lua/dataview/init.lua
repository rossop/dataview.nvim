-- dataview.nvim: run Obsidian Dataview queries in Neovim.
-- Thin client: block detection and layout here, everything else in the Go
-- binary (§V.9).
local config = require("dataview.config")

local M = {}

local is_windows = vim.fn.has("win32") == 1
local exe = is_windows and "dataview.exe" or "dataview"
local plugin_root = vim.fn.fnamemodify(debug.getinfo(1, "S").source:sub(2), ":h:h:h")

function M.setup(opts)
  config.setup(opts)
end

--- Where the binary comes from, in order: opts.binary, a local build in the
--- plugin's bin/, the auto-installed copy, then $PATH.
function M.binary()
  local o = config.options
  if o.binary then
    return vim.fn.expand(o.binary)
  end
  for _, p in ipairs({
    plugin_root .. "/bin/" .. exe,
    vim.fn.stdpath("data") .. "/dataview/bin/" .. exe,
  }) do
    if vim.fn.executable(p) == 1 then
      return p
    end
  end
  if vim.fn.executable(exe) == 1 then
    return vim.fn.exepath(exe)
  end
  return nil
end

--- Vault root for a buffer: opts.vault, else the nearest folder with .obsidian/.
function M.vault(buf)
  local o = config.options
  if o.vault then
    return (vim.fs.normalize(vim.fn.fnamemodify(vim.fn.expand(o.vault), ":p")):gsub("/$", ""))
  end
  local name = vim.api.nvim_buf_get_name(buf or 0)
  if name == "" then
    return nil
  end
  return vim.fs.root(name, ".obsidian")
end

local function notify(msg, level)
  vim.notify("dataview: " .. msg, level or vim.log.levels.WARN)
end

local function in_vault(vault, path)
  return path ~= "" and vim.startswith(vim.fs.normalize(path), vault .. "/")
end

local shown = {} -- buf -> true while results are displayed

local function request(buf, method, params, cb)
  local vault = M.vault(buf)
  if not vault then
    return cb({ message = "no vault found: open a note inside a folder with .obsidian/ or set vault in setup()" })
  end
  local bin = M.binary()
  if not bin then
    return cb({ message = "binary not found; run :checkhealth dataview" })
  end
  require("dataview.process").request(vault, bin, method, params, cb)
end

--- Run every dataview block in the buffer and show results inline.
function M.run(buf)
  buf = (buf == nil or buf == 0) and vim.api.nvim_get_current_buf() or buf
  local blocks = require("dataview.blocks").find(buf)
  local inline = require("dataview.render.inline")
  if #blocks == 0 then
    inline.clear(buf)
    shown[buf] = nil
    return 0
  end
  shown[buf] = true
  local opts = config.options
  for _, block in ipairs(blocks) do
    if not vim.b[buf].dataview_ran then
      inline.show(buf, block, { { { "dataview: running…", "DataviewMuted" } } })
    end
    local started = vim.uv.hrtime()
    request(buf, "query", { dql = block.query }, function(err, res)
      if not shown[buf] then
        return -- toggled off while the query was in flight
      end
      local ms = math.floor((vim.uv.hrtime() - started) / 1e6)
      inline.show(buf, block, inline.format(res, err and err.message, opts, ms))
    end)
  end
  vim.b[buf].dataview_ran = true
  return #blocks
end

function M.clear(buf)
  buf = (buf == nil or buf == 0) and vim.api.nvim_get_current_buf() or buf
  shown[buf] = nil
  require("dataview.render.inline").clear(buf)
end

function M.toggle(buf)
  buf = (buf == nil or buf == 0) and vim.api.nvim_get_current_buf() or buf
  if shown[buf] then
    M.clear(buf)
  else
    M.run(buf)
  end
end

function M.is_shown(buf)
  return shown[buf] == true
end

--- Tell the binary a file changed, then re-run results that may depend on it.
function M.on_write(buf)
  local path = vim.api.nvim_buf_get_name(buf)
  local vault = M.vault(buf)
  if not vault or not in_vault(vault, path) then
    return
  end
  request(buf, "refresh", { paths = { path } }, function(err)
    if err then
      return require("dataview.process").log("refresh failed: " .. err.message)
    end
    if not config.options.refresh_on_save then
      return
    end
    for b in pairs(shown) do
      if vim.api.nvim_buf_is_valid(b) and M.vault(b) == vault then
        M.run(b)
      else
        shown[b] = nil
      end
    end
  end)
end

--- Rescan the whole vault (after bulk changes outside Neovim).
function M.refresh(buf)
  buf = (buf == nil or buf == 0) and vim.api.nvim_get_current_buf() or buf
  request(buf, "refresh", vim.empty_dict(), function(err, res)
    if err then
      return notify(err.message)
    end
    notify(("indexed %d notes"):format(res.notes), vim.log.levels.INFO)
    if shown[buf] then
      M.run(buf)
    end
  end)
end

function M.log()
  local lines = require("dataview.process").log_lines()
  vim.cmd("botright new")
  local b = vim.api.nvim_get_current_buf()
  vim.bo[b].buftype = "nofile"
  vim.bo[b].bufhidden = "wipe"
  vim.api.nvim_buf_set_name(b, "dataview://log")
  vim.api.nvim_buf_set_lines(b, 0, -1, false, #lines > 0 and lines or { "(empty)" })
  vim.bo[b].modifiable = false
end

-- Auto-render only spends a process on notes that actually contain blocks.
function M.on_enter(buf)
  if not config.options.auto or shown[buf] or vim.b[buf].dataview_ran then
    return
  end
  if require("dataview.blocks").has_blocks(buf) and M.vault(buf) then
    M.run(buf)
  end
end

M._request = request
M._notify = notify
return M
