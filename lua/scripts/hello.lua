function handle_message(event)
  if event.content == "!ping" then
    send_message(event.channel_id, "Pong!")
  end
end

register_hook("message_create", handle_message)

