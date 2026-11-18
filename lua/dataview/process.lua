-- Async JSON-RPC client for `dataview serve`. One process per vault, started
-- on first use and stopped with Neovim. Nothing here blocks the UI (§V.8):
-- replies arrive on libuv callbacks and are handed to the main loop with
-- vim.schedule.
local M = {}

-- Must equal rpc.ProtocolVersion in the Go binary (§V.11).
M.PROTOCOL_VERSION = 1

local MAX_RESTARTS = 1
local clients = {}
local log_lines = {}

local function log(msg)
  log_lines[#log_lines + 1] = os.date("%H:%M:%S ") .. msg
  if #log_lines > 500 then
    table.remove(log_lines, 1)
  end
end
M.log = log

function M.log_lines()
  return log_lines
end

local Client = {}
Client.__index = Client

local function new_client(vault, binary, restarts)
  return setmetatable({
    vault = vault,
    binary = binary,
    restarts = restarts or 0,
    next_id = 0,
    pending = {},
    queue = {},
    partial = "",
    ready = false,
    alive = false,
    failed = nil,
  }, Client)
end

function Client:start()
  local ok, proc = pcall(vim.system, { self.binary, "serve" }, {
    stdin = true,
    stdout = function(_, data)
      if data then
        self:on_stdout(data)
      end
    end,
    stderr = function(_, data)
      if data then
        vim.schedule(function()
          for line in data:gmatch("[^\r\n]+") do
            log(line)
          end
        end)
      end
    end,
  }, function(obj)
    vim.schedule(function()
      self:on_exit(obj.code, obj.signal)
    end)
  end)
  if not ok then
    self:fail(("cannot start %s: %s"):format(self.binary, proc))
    return
  end
  self.proc = proc
  self.alive = true
  log(("started %s serve for %s (pid %d)"):format(self.binary, self.vault, proc.pid))

  self:send_now("initialize", { vault = self.vault }, function(err, res)
    if err then
      return self:fail("initialize failed: " .. err.message)
    end
    if res.protocolVersion ~= M.PROTOCOL_VERSION then
      return self:fail(
        ("binary %s speaks protocol %s but the plugin needs %s; update the binary or run :checkhealth dataview"):format(
          res.version,
          tostring(res.protocolVersion),
          M.PROTOCOL_VERSION
        )
      )
    end
    self.ready = true
    self.info = res
    log(("indexed %d notes (binary %s)"):format(res.notes, res.version))
    local queued = self.queue
    self.queue = {}
    for _, q in ipairs(queued) do
      self:send_now(q[1], q[2], q[3])
    end
  end)
end

-- Runs on the libuv thread: only buffer bytes here, decode on the main loop.
function Client:on_stdout(data)
  local text = self.partial .. data
  local lines = {}
  local last = 1
  for line, stop in text:gmatch("([^\n]*)\n()") do
    lines[#lines + 1] = line
    last = stop
  end
  self.partial = text:sub(last)
  if #lines > 0 then
    vim.schedule(function()
      for _, line in ipairs(lines) do
        self:on_message(line)
      end
    end)
  end
end

function Client:on_message(line)
  if line == "" then
    return
  end
  local ok, msg = pcall(vim.json.decode, line, { luanil = { object = true, array = true } })
  if not ok or type(msg) ~= "table" then
    log("bad message from binary: " .. line)
    return
  end
  local cb = self.pending[msg.id]
  if not cb then
    return
  end
  self.pending[msg.id] = nil
  cb(msg.error, msg.result)
end

function Client:on_exit(code, signal)
  self.alive = false
  self.ready = false
  local reason = ("dataview exited (code %s, signal %s)"):format(tostring(code), tostring(signal))
  log(reason)
  if not self.stopping then
    self.restarts = self.restarts + 1
  end
  self:flush_errors(self.failed or (reason .. "; see :Dataview log"))
end

function Client:flush_errors(message)
  local pending, queue = self.pending, self.queue
  self.pending, self.queue = {}, {}
  for _, cb in pairs(pending) do
    cb({ message = message })
  end
  for _, q in ipairs(queue) do
    q[3]({ message = message })
  end
end

function Client:fail(message)
  log(message)
  self.failed = message
  self.stopping = true -- a misconfiguration will not fix itself by restarting
  self:flush_errors(message)
  if self.proc then
    pcall(self.proc.kill, self.proc, "sigterm")
  end
end

function Client:send_now(method, params, cb)
  self.next_id = self.next_id + 1
  local id = self.next_id
  self.pending[id] = cb
  local msg = { jsonrpc = "2.0", id = id, method = method }
  if params ~= nil then
    msg.params = params
  end
  local ok, err = pcall(self.proc.write, self.proc, vim.json.encode(msg) .. "\n")
  if not ok then
    self.pending[id] = nil
    cb({ message = "write to dataview failed: " .. tostring(err) })
  end
end

function Client:request(method, params, cb)
  if self.failed then
    return cb({ message = self.failed })
  end
  if self.ready then
    self:send_now(method, params, cb)
  else
    self.queue[#self.queue + 1] = { method, params, cb }
  end
end

function Client:stop()
  if not self.alive then
    return
  end
  self.stopping = true
  -- Closing stdin makes the server save its cache and exit.
  pcall(self.proc.write, self.proc, nil)
  pcall(self.proc.wait, self.proc, 1000)
end

--- Send a request to the server for `vault`, starting it if needed.
--- cb(err, result) always runs on the main loop.
function M.request(vault, binary, method, params, cb)
  local c = clients[vault]
  if not c or (not c.alive and not c.failed) then
    local restarts = c and c.restarts or 0
    if restarts > MAX_RESTARTS then
      return cb({ message = "dataview keeps crashing; see :Dataview log" })
    end
    c = new_client(vault, binary, restarts)
    clients[vault] = c
    c:start()
  end
  c:request(method, params, cb)
end

function M.info(vault)
  local c = clients[vault]
  return c and c.info
end

function M.stop_all()
  for vault, c in pairs(clients) do
    c:stop()
    clients[vault] = nil
  end
end

-- Forget a failed client so the next request tries again (e.g. after the
-- user fixed the binary path).
function M.reset(vault)
  local c = clients[vault]
  if c then
    c:stop()
  end
  clients[vault] = nil
end

return M
