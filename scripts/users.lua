-- User management commands

if get_owner() ~= nil then return end

register_command("claim_admin", "Claim admin access with a one-time token (DM only)", function(event)
    if event.guild_id ~= "" then
        send_message(event.channel_id, "This command only works in DMs.")
        return
    end

    local token = event.args[2]
    if not token then
        send_message(event.channel_id, "Usage: !claim_admin <token>")
        return
    end

    local ok = user_claim_admin(event.author_id, event.author, token)
    if ok then
        send_message(event.channel_id, "Admin access granted.")
        log("ADMIN BOOTSTRAP: " .. event.author .. " (" .. event.author_id .. ") claimed admin.")
        unregister_command("claim_admin")
    else
        send_message(event.channel_id, "Invalid or expired token.")
    end
end)
