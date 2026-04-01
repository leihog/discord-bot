
register_command("dadjoke", "Fetch a random dad joke", function(event)
    log("Fetching joke for " .. event.author .. " (async)...")

    http_get_async("https://icanhazdadjoke.com", {
        headers = { ["Accept"] = "application/json" },
        timeout = 10
    }, function(result)
        if result.error then
            send_message(event.channel_id, "Failed to fetch joke: " .. result.error)
            return
        end
        if result.status ~= 200 then
            send_message(event.channel_id, "API returned status " .. result.status)
            return
        end

        local data = json_decode(result.body)
        if data and data.joke then
            send_message(event.channel_id, data.joke)
        else
            send_message(event.channel_id, "Couldn't parse the joke response.")
        end
    end)
end, 5)