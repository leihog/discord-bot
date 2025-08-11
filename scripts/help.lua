function handle_help(event)
    local commands = get_commands()
    local helpText = "**Available Commands:**\n"
    
    for name, cmd in pairs(commands) do
        local cooldownText = ""
        if cmd.cooldown > 0 then
            cooldownText = " (cooldown: " .. cmd.cooldown .. "s)"
        end
        helpText = helpText .. "• `!" .. name .. "` - " .. cmd.description .. cooldownText .. "\n"
    end
    
    send_message(event.channel_id, helpText)
end

register_command("cmds", "Shows all available commands", handle_help, 30) 