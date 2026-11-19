local M = {}

function M.check()
  local health = vim.health
  local dv = require("dataview")
  local config = require("dataview.config").options
  local install = require("dataview.install")
  local want = require("dataview.version")

  health.start("dataview.nvim")
  if vim.fn.has("nvim-0.10") == 1 then
    health.ok("Neovim " .. tostring(vim.version()))
  else
    health.error("Neovim 0.10+ required")
  end

  local bin = dv.binary()
  if not bin then
    health.error("dataview binary not found", {
      "Run :Dataview install (downloads a checksum-verified release binary)",
      "or build it: go build -o bin/dataview ./cmd/dataview (inside the plugin folder)",
      "or set binary = '/path/to/dataview' in setup()",
    })
  else
    local v = install.binary_version(bin)
    if not v then
      health.error(bin .. " does not run", { "Reinstall with :Dataview install" })
    elseif v ~= want and v ~= "dev" then
      health.warn(("binary %s is v%s, plugin expects v%s"):format(bin, v, want), { "Run :Dataview install" })
    else
      health.ok(("binary %s (v%s)"):format(bin, v))
    end
  end

  -- :checkhealth runs from its own buffer; the note it was called from is
  -- the alternate buffer.
  local src = vim.fn.bufnr("#")
  local vault = dv.vault(src > 0 and src or 0)
  if vault then
    health.ok("vault: " .. vault .. (config.vault and " (from setup)" or " (nearest .obsidian/)"))
    local info = require("dataview.process").info(vault)
    if info then
      health.ok(("server running: %d notes, protocol %d"):format(info.notes, info.protocolVersion))
    else
      health.info("server not started yet (starts on first query)")
    end
  else
    health.warn("no vault for the current buffer", {
      "Open a note inside a folder that contains .obsidian/",
      "or set vault = '~/path/to/vault' in setup()",
    })
  end

  if pcall(vim.treesitter.language.inspect, "markdown") then
    health.ok("Treesitter markdown parser (block detection)")
  else
    health.info("no Treesitter markdown parser; using the built-in line scanner")
  end

  local pickers = {}
  for _, p in ipairs({ { "telescope", "telescope" }, { "fzf-lua", "fzf-lua" }, { "snacks", "snacks" } }) do
    if pcall(require, p[2]) then
      pickers[#pickers + 1] = p[1]
    end
  end
  health.info("pickers: " .. (#pickers > 0 and table.concat(pickers, ", ") or "vim.ui.select only (that's fine)"))
end

return M
