return function(items, ctx)
  local entries = {}
  for i, item in ipairs(items) do
    entries[i] = { idx = i, text = item.text, file = item.file, pos = { item.lnum, 0 }, dv = item }
  end
  require("snacks").picker.pick({
    title = ctx.title,
    items = entries,
    format = function(e)
      return { { e.text } }
    end,
    confirm = function(picker, e)
      picker:close()
      if e then
        ctx.open(e.dv.row)
      end
    end,
  })
end
