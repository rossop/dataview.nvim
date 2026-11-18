-- Find ```dataview fenced blocks in a buffer. Uses the Treesitter markdown
-- parser when present (handles fences inside lists and quotes correctly) and
-- falls back to a line scanner so the plugin works with zero setup.
local M = {}

local function ts_blocks(buf)
  local ok, parser = pcall(vim.treesitter.get_parser, buf, "markdown")
  if not ok or not parser then
    return nil
  end
  local tree = parser:parse()[1]
  if not tree then
    return nil
  end
  local found = {}
  local function visit(node)
    for child in node:iter_children() do
      if child:type() == "fenced_code_block" then
        local lang, content
        for part in child:iter_children() do
          if part:type() == "info_string" then
            lang = vim.trim(vim.treesitter.get_node_text(part, buf))
          elseif part:type() == "code_fence_content" then
            content = vim.treesitter.get_node_text(part, buf)
          end
        end
        if lang and lang:lower() == "dataview" then
          local sr, _, er, ec = child:range()
          -- The node ends at column 0 of the line after the closing fence.
          if ec == 0 and er > sr then
            er = er - 1
          end
          found[#found + 1] = { start_row = sr, end_row = er, query = vim.trim(content or "") }
        end
      elseif child:named_child_count() > 0 then
        visit(child)
      end
    end
  end
  visit(tree:root())
  return found
end

--- Line-scanner fallback. Exposed for tests.
function M.scan_lines(lines)
  local found = {}
  local open -- { row, fence, indent }
  local body = {}
  for i, line in ipairs(lines) do
    local row = i - 1
    if not open then
      local indent, fence, info = line:match("^(%s*)([`~][`~][`~]+)%s*(%S*)%s*$")
      if fence and fence:sub(1, 1):rep(#fence) == fence then
        open = { row = row, fence = fence, dataview = info:lower() == "dataview" }
        body = {}
      end
    else
      local fence = line:match("^%s*([`~][`~][`~]+)%s*$")
      if fence and fence:sub(1, 1) == open.fence:sub(1, 1) and #fence >= #open.fence then
        if open.dataview then
          found[#found + 1] = { start_row = open.row, end_row = row, query = vim.trim(table.concat(body, "\n")) }
        end
        open = nil
      else
        body[#body + 1] = line
      end
    end
  end
  return found
end

--- Returns a list of { start_row, end_row, query } (0-based rows; end_row is
--- the closing fence). Unclosed blocks are ignored.
function M.find(buf)
  local blocks = ts_blocks(buf)
  if blocks then
    return blocks
  end
  return M.scan_lines(vim.api.nvim_buf_get_lines(buf, 0, -1, false))
end

--- Cheap check used before starting the binary for auto-rendering.
function M.has_blocks(buf)
  for _, line in ipairs(vim.api.nvim_buf_get_lines(buf, 0, -1, false)) do
    if line:match("^%s*[`~][`~][`~]+%s*[Dd][Aa][Tt][Aa][Vv][Ii][Ee][Ww]%s*$") then
      return true
    end
  end
  return false
end

return M
