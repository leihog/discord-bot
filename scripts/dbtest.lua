
function handle_message(event)
    if event.content == "!dbtest" then
        local user = store_get("stats.users", event.author) or { count = 0 }
        user.count = user.count + 1
        store_set("stats.users", event.author, user)

        send_message(event.channel_id, "Seen you " .. user.count .. " times! " .. event.author)
    end
end

register_hook("on_direct_message", handle_message)