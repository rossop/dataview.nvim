return function(items, ctx)
  vim.ui.select(items, {
    prompt = ctx.title,
    format_item = function(item)
      return item.text
    end,
  }, function(item)
    if item then
      ctx.open(item.row)
    end
  end)
end
