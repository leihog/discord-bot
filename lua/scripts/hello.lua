greeting_counter = 0

function handle_message(event)
  if event.content:lower():match("^hej") then
    send_message(event.channel_id, "Hej, " .. event.author .. "!")
    greeting_counter = greeting_counter + 1
  
  elseif event.content:match("^!greet_count") then
    send_message(event.channel_id, "I've said hej " .. greeting_counter .. " times!")
  end
end

register_hook("on_channel_message", handle_message)
register_hook("on_direct_message", handle_message)
