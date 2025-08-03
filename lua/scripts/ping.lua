function handle_ping(event)
  send_message(event.channel_id, "Pong!")
end

register_command("ping", "Replies with Pong!", handle_ping, 10)