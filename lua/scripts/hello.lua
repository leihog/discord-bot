function handle_message(event)
  if event.content:lower() == "hej" then
    send_message(event.channel_id, "Hej, " .. event.author .. "!")
  end
end

register_hook("on_channel_message", handle_message)
register_hook("on_direct_message", handle_message)
