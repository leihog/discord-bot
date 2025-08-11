
function handle_message(event)
  if event.content:lower():match("^hello") then
    send_message(event.channel_id, "Hello, " .. event.author .. "!")
  end
end

register_hook("on_channel_message", handle_message)
register_hook("on_direct_message", handle_message)