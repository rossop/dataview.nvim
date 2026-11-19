return function(items, ctx)
  local pickers = require("telescope.pickers")
  local finders = require("telescope.finders")
  local conf = require("telescope.config").values
  pickers
    .new({}, {
      prompt_title = ctx.title,
      finder = finders.new_table({
        results = items,
        entry_maker = function(item)
          return {
            value = item,
            display = item.text,
            ordinal = item.text,
            filename = item.file,
            path = item.file,
            lnum = item.lnum,
          }
        end,
      }),
      sorter = conf.generic_sorter({}),
      previewer = conf.grep_previewer({}),
    })
    :find()
end
