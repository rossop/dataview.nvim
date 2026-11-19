return function(items, ctx)
  local fzf = require("fzf-lua")
  -- fzf works on strings; number them so the choice maps back to its item
  -- even when two rows display the same text.
  local lines, by_line = {}, {}
  for i, item in ipairs(items) do
    local line = ("%d\t%s"):format(i, item.text)
    lines[i] = line
    by_line[line] = item
  end
  fzf.fzf_exec(lines, {
    prompt = ctx.title .. "> ",
    fzf_opts = { ["--delimiter"] = "\t", ["--with-nth"] = "2.." },
    actions = {
      ["default"] = function(selected)
        local item = selected and by_line[selected[1]]
        if item then
          ctx.open(item.row)
        end
      end,
    },
  })
end
