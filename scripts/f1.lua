-- Formula 1 commands
-- Data from the Jolpica API (https://api.jolpi.ca/ergast/f1/)
--
-- Commands:
--   !f1 leaderboard      — top 10 driver championship standings
--   !f1 constructor      — constructor/team championship standings
--   !f1 last             — top 10 results from the most recent race
--   !f1 next race        — date and location of the next race
--   !f1 races            — all upcoming races this season
--   !f1 qualifying       — last qualifying grid (also: !f1 quali)
--   !f1 podiums          — podium finishers for each completed race
--   !f1 driver <name>    — stats for a specific driver
--   !f1 teammates        — side-by-side teammate comparison
--   !f1 set channel      — set this channel for announcements & reminders
--   !f1 reminders on|off — toggle pre-race reminder (~1 hour before start)
--   !f1 announce on|off  — toggle automatic podium announcement after each race

local CACHE_TTL      = 900   -- 15 minutes (standings, schedule, podiums)
local LIVE_TTL       = 300   -- 5 minutes (last race, qualifying — more likely to change)
local API_BASE       = "https://api.jolpi.ca/ergast/f1/current"
local CACHE_NS       = "f1_cache"
local SETTINGS_NS    = "f1_settings"
local CHECK_INTERVAL = 300   -- background check every 5 minutes

local MEDAL = { [1] = "🥇", [2] = "🥈", [3] = "🥉" }

-- ── helpers ───────────────────────────────────────────────────────────────────

-- Fetch JSON from url, caching the decoded result in KV store for ttl seconds.
-- Calls callback(data) on success or callback(nil, err_string) on failure.
local function fetch_cached(cache_key, url, ttl, callback)
    local cached = store_get(CACHE_NS, cache_key)
    if cached and cached.expires and os.time() < cached.expires then
        callback(cached.data)
        return
    end
    http_get_async(url, { timeout = 15 }, function(res)
        if res.error then
            callback(nil, "HTTP error: " .. res.error)
            return
        end
        if res.status ~= 200 then
            callback(nil, "API returned status " .. tostring(res.status))
            return
        end
        local data = json_decode(res.body)
        if not data then
            callback(nil, "Failed to parse API response")
            return
        end
        store_set(CACHE_NS, cache_key, { data = data, expires = os.time() + ttl })
        callback(data)
    end)
end

-- Send lines as one or more messages, respecting Discord's 2000-char limit.
local function send_lines(channel_id, lines)
    local MAX = 1900
    local chunk, len = {}, 0
    for _, line in ipairs(lines) do
        local llen = #line + 1
        if len + llen > MAX and #chunk > 0 then
            send_message(channel_id, table.concat(chunk, "\n"))
            chunk, len = {}, 0
        end
        chunk[#chunk + 1] = line
        len = len + llen
    end
    if #chunk > 0 then
        send_message(channel_id, table.concat(chunk, "\n"))
    end
end

-- "20:00:00Z" → "20:00 UTC"
local function fmt_time(t)
    local hm = t and t:match("^(%d%d:%d%d)")
    return hm and (hm .. " UTC") or (t or "?")
end

-- Current date in UTC as "YYYY-MM-DD"
local function utc_date()
    return os.date("!%Y-%m-%d")
end

-- Current UTC time as minutes since midnight
local function utc_minutes_now()
    return tonumber(os.date("!%H")) * 60 + tonumber(os.date("!%M"))
end

-- "HH:MM:SSZ" → minutes since midnight
local function time_to_minutes(t)
    local h, m = t:match("^(%d+):(%d+)")
    if not h then return nil end
    return tonumber(h) * 60 + tonumber(m)
end

-- ── settings helpers ──────────────────────────────────────────────────────────

local function get_setting(key)
    return store_get(SETTINGS_NS, key)
end

local function set_setting(key, value)
    store_set(SETTINGS_NS, key, value)
end

-- ── leaderboard ───────────────────────────────────────────────────────────────

local function cmd_leaderboard(channel_id)
    fetch_cached("standings", API_BASE .. "/driverstandings.json", CACHE_TTL, function(data, err)
        if not data then
            send_message(channel_id, "Could not fetch standings. " .. (err or ""))
            return
        end
        local lists = data.MRData.StandingsTable.StandingsLists
        if not lists or #lists == 0 then
            send_message(channel_id, "No standings available yet — check back once the season is underway!")
            return
        end
        local list  = lists[1]
        local lines = { string.format("**F1 %s Driver Standings** (after Round %s)", list.season, list.round) }
        for i, e in ipairs(list.DriverStandings) do
            if i > 10 then break end
            local pos   = tonumber(e.position) or i
            local name  = e.Driver.givenName .. " " .. e.Driver.familyName
            local team  = (e.Constructors and e.Constructors[1] and e.Constructors[1].name) or "?"
            local badge = MEDAL[pos] or string.format("%2d.", pos)
            lines[#lines + 1] = string.format("%s %s (%s) — %s pts", badge, name, team, e.points)
        end
        send_message(channel_id, table.concat(lines, "\n"))
    end)
end

-- ── constructor standings ─────────────────────────────────────────────────────

local function cmd_constructor(channel_id)
    fetch_cached("constructors", API_BASE .. "/constructorstandings.json", CACHE_TTL, function(data, err)
        if not data then
            send_message(channel_id, "Could not fetch constructor standings. " .. (err or ""))
            return
        end
        local lists = data.MRData.StandingsTable.StandingsLists
        if not lists or #lists == 0 then
            send_message(channel_id, "No constructor standings available yet — check back once the season is underway!")
            return
        end
        local list  = lists[1]
        local lines = { string.format("**F1 %s Constructor Standings** (after Round %s)", list.season, list.round) }
        for i, e in ipairs(list.ConstructorStandings) do
            local pos   = tonumber(e.position) or i
            local badge = MEDAL[pos] or string.format("%2d.", pos)
            lines[#lines + 1] = string.format("%s %s — %s pts (%s win%s)",
                badge, e.Constructor.name, e.points, e.wins, e.wins == "1" and "" or "s")
        end
        send_message(channel_id, table.concat(lines, "\n"))
    end)
end

-- ── last race results ─────────────────────────────────────────────────────────

local function cmd_last(channel_id)
    fetch_cached("last_race", API_BASE .. "/last/results.json", LIVE_TTL, function(data, err)
        if not data then
            send_message(channel_id, "Could not fetch last race results. " .. (err or ""))
            return
        end
        local races = data.MRData.RaceTable.Races
        if not races or #races == 0 then
            send_message(channel_id, "No race results yet — the season hasn't started!")
            return
        end
        local race  = races[1]
        local lines = { string.format("**%s — Round %s (%s)**", race.raceName, race.round, race.date) }
        for i, e in ipairs(race.Results or {}) do
            if i > 10 then break end
            local pos   = tonumber(e.position) or i
            local name  = e.Driver.givenName .. " " .. e.Driver.familyName
            local team  = (e.Constructor and e.Constructor.name) or "?"
            local badge = MEDAL[pos] or string.format("%2d.", pos)
            local time  = (e.Time and e.Time.time) or e.status or "?"
            lines[#lines + 1] = string.format("%s %s (%s) — %s", badge, name, team, time)
        end
        send_message(channel_id, table.concat(lines, "\n"))
    end)
end

-- ── next race ─────────────────────────────────────────────────────────────────

local function cmd_next_race(channel_id)
    fetch_cached("next_race", API_BASE .. "/next.json", CACHE_TTL, function(data, err)
        if not data then
            send_message(channel_id, "Could not fetch next race. " .. (err or ""))
            return
        end
        local races = data.MRData.RaceTable.Races
        if not races or #races == 0 then
            send_message(channel_id, "No upcoming races — the season may be over. Stay tuned for next year!")
            return
        end
        local r        = races[1]
        local loc      = r.Circuit.Location
        local time_str = r.time and (" at " .. fmt_time(r.time)) or ""
        send_message(channel_id, string.format(
            "**Next Race: %s (Round %s)**\n📅 %s%s\n🏎️  %s, %s, %s",
            r.raceName, r.round, r.date, time_str,
            r.Circuit.circuitName, loc.locality, loc.country
        ))
    end)
end

-- ── upcoming races ────────────────────────────────────────────────────────────

local function cmd_races(channel_id)
    fetch_cached("schedule", API_BASE .. ".json", CACHE_TTL, function(data, err)
        if not data then
            send_message(channel_id, "Could not fetch schedule. " .. (err or ""))
            return
        end
        local all    = data.MRData.RaceTable.Races
        local season = data.MRData.RaceTable.season
        if not all or #all == 0 then
            send_message(channel_id, "No race schedule available yet!")
            return
        end
        local today, upcoming = utc_date(), {}
        for _, r in ipairs(all) do
            if r.date >= today then upcoming[#upcoming + 1] = r end
        end
        if #upcoming == 0 then
            send_message(channel_id, string.format(
                "No upcoming races — the %s season is over. Stay tuned for next year!", season))
            return
        end
        local lines = { string.format("**Upcoming %s F1 Races** (%d remaining)", season, #upcoming) }
        for _, r in ipairs(upcoming) do
            lines[#lines + 1] = string.format("Rd %s — %s — %s (%s, %s)",
                r.round, r.raceName, r.date, r.Circuit.circuitName, r.Circuit.Location.country)
        end
        send_lines(channel_id, lines)
    end)
end

-- ── qualifying ────────────────────────────────────────────────────────────────

local function cmd_qualifying(channel_id)
    fetch_cached("qualifying", API_BASE .. "/last/qualifying.json", LIVE_TTL, function(data, err)
        if not data then
            send_message(channel_id, "Could not fetch qualifying results. " .. (err or ""))
            return
        end
        local races = data.MRData.RaceTable.Races
        if not races or #races == 0 then
            send_message(channel_id, "No qualifying results available yet!")
            return
        end
        local race  = races[1]
        local lines = { string.format("**Qualifying — %s (Round %s)**", race.raceName, race.round) }
        for i, e in ipairs(race.QualifyingResults or {}) do
            local pos   = tonumber(e.position) or i
            local name  = e.Driver.givenName .. " " .. e.Driver.familyName
            local team  = (e.Constructor and e.Constructor.name) or "?"
            local badge = MEDAL[pos] or string.format("%2d.", pos)
            local best  = e.Q3 or e.Q2 or e.Q1 or "?"
            lines[#lines + 1] = string.format("%s %s (%s) — %s", badge, name, team, best)
        end
        send_lines(channel_id, lines)
    end)
end

-- ── podiums ───────────────────────────────────────────────────────────────────

local function cmd_podiums(channel_id)
    fetch_cached("results", API_BASE .. "/results.json?limit=500", CACHE_TTL, function(data, err)
        if not data then
            send_message(channel_id, "Could not fetch results. " .. (err or ""))
            return
        end
        local races  = data.MRData.RaceTable.Races
        local season = data.MRData.RaceTable.season
        if not races or #races == 0 then
            send_message(channel_id, "No race results yet — the season hasn't started!")
            return
        end
        local lines = { string.format("**%s F1 Race Podiums**", season) }
        for _, race in ipairs(races) do
            lines[#lines + 1] = string.format("Rd %s — %s", race.round, race.raceName)
            local shown = 0
            for _, e in ipairs(race.Results or {}) do
                local pos = tonumber(e.position)
                if pos and pos <= 3 then
                    lines[#lines + 1] = string.format("  %s %s",
                        MEDAL[pos] or tostring(pos),
                        e.Driver.givenName .. " " .. e.Driver.familyName)
                    shown = shown + 1
                    if shown == 3 then break end
                end
            end
        end
        send_lines(channel_id, lines)
    end)
end

-- ── driver lookup ─────────────────────────────────────────────────────────────

local function cmd_driver(channel_id, query)
    if not query or query == "" then
        send_message(channel_id, "Usage: `!f1 driver <name>` — e.g. `!f1 driver Hamilton` or `!f1 driver HAM`")
        return
    end
    fetch_cached("standings", API_BASE .. "/driverstandings.json", CACHE_TTL, function(data, err)
        if not data then
            send_message(channel_id, "Could not fetch standings. " .. (err or ""))
            return
        end
        local lists = data.MRData.StandingsTable.StandingsLists
        if not lists or #lists == 0 then
            send_message(channel_id, "No standings data available yet.")
            return
        end
        local q     = query:lower()
        local match = nil
        for _, e in ipairs(lists[1].DriverStandings) do
            local full = (e.Driver.givenName .. " " .. e.Driver.familyName):lower()
            local code = (e.Driver.code or ""):lower()
            if full:find(q, 1, true) or code == q then
                match = e
                break
            end
        end
        if not match then
            send_message(channel_id, string.format("No driver found matching \"%s\".", query))
            return
        end
        local d    = match.Driver
        local team = (match.Constructors and match.Constructors[1] and match.Constructors[1].name) or "?"
        send_message(channel_id, string.format(
            "**%s %s** (#%s, %s)\nChampionship: P%s — %s pts — %s win%s\nNationality: %s",
            d.givenName, d.familyName,
            d.permanentNumber or "?", team,
            match.position, match.points, match.wins,
            match.wins == "1" and "" or "s",
            d.nationality or "?"
        ))
    end)
end

-- ── teammates ─────────────────────────────────────────────────────────────────

local function cmd_teammates(channel_id)
    fetch_cached("standings", API_BASE .. "/driverstandings.json", CACHE_TTL, function(data, err)
        if not data then
            send_message(channel_id, "Could not fetch standings. " .. (err or ""))
            return
        end
        local lists = data.MRData.StandingsTable.StandingsLists
        if not lists or #lists == 0 then
            send_message(channel_id, "No standings data available yet.")
            return
        end
        local list = lists[1]
        -- Group drivers by constructor, preserving first-seen order
        local teams, order = {}, {}
        for _, e in ipairs(list.DriverStandings) do
            local team = (e.Constructors and e.Constructors[1] and e.Constructors[1].name) or "Unknown"
            if not teams[team] then
                teams[team] = {}
                order[#order + 1] = team
            end
            teams[team][#teams[team] + 1] = e
        end
        local lines = { string.format("**F1 %s Teammate Comparison** (after Round %s)", list.season, list.round) }
        for _, team in ipairs(order) do
            lines[#lines + 1] = string.format("**%s**", team)
            for _, e in ipairs(teams[team]) do
                local pos   = tonumber(e.position) or 0
                local name  = e.Driver.givenName .. " " .. e.Driver.familyName
                local badge = MEDAL[pos] or string.format("%2d.", pos)
                lines[#lines + 1] = string.format("  %s %s — %s pts, %s win%s",
                    badge, name, e.points, e.wins, e.wins == "1" and "" or "s")
            end
        end
        send_lines(channel_id, lines)
    end)
end

-- ── settings commands ─────────────────────────────────────────────────────────

local function cmd_set_channel(channel_id)
    set_setting("channel", channel_id)
    send_message(channel_id, "✅ This channel will receive F1 race announcements and reminders (when enabled).\n" ..
        "Use `!f1 reminders on` and `!f1 announce on` to activate them.")
end

local function cmd_reminders(channel_id, state)
    if state == "on" then
        set_setting("reminders", true)
        send_message(channel_id, "✅ Race reminders enabled — I'll post a heads-up ~1 hour before each race.")
    elseif state == "off" then
        set_setting("reminders", false)
        send_message(channel_id, "🔕 Race reminders disabled.")
    else
        local on = get_setting("reminders")
        send_message(channel_id, "Race reminders are currently " .. (on and "**on**" or "**off**") ..
            ". Use `!f1 reminders on` or `!f1 reminders off` to change.")
    end
end

local function cmd_announce(channel_id, state)
    if state == "on" then
        set_setting("announce", true)
        send_message(channel_id, "✅ Race result announcements enabled — I'll post the podium after each race.")
    elseif state == "off" then
        set_setting("announce", false)
        send_message(channel_id, "🔕 Race result announcements disabled.")
    else
        local on = get_setting("announce")
        send_message(channel_id, "Race announcements are currently " .. (on and "**on**" or "**off**") ..
            ". Use `!f1 announce on` or `!f1 announce off` to change.")
    end
end

-- ── background checks ─────────────────────────────────────────────────────────

-- Sends a reminder if the next race starts within the 50–70 minute window
-- (catches the one 5-minute check that falls in the ~1 hour slot).
local function do_race_reminder(channel_id)
    fetch_cached("next_race", API_BASE .. "/next.json", CACHE_TTL, function(data)
        if not data then return end
        local races = data.MRData.RaceTable.Races
        if not races or #races == 0 then return end
        local r = races[1]
        if not r.time then return end

        local last = get_setting("last_reminder_round")
        if r.round == last then return end  -- already reminded for this race

        if r.date ~= utc_date() then return end  -- not today

        local race_min = time_to_minutes(r.time)
        local now_min  = utc_minutes_now()
        local diff     = race_min - now_min

        if diff >= 50 and diff <= 70 then
            local loc = r.Circuit.Location
            send_message(channel_id, string.format(
                "🏁 **Race starting in ~1 hour!**\n%s (Round %s) — %s, %s\nStarts at %s",
                r.raceName, r.round, loc.locality, loc.country, fmt_time(r.time)
            ))
            set_setting("last_reminder_round", r.round)
        end
    end)
end

-- Announces the podium the first time new race results are detected.
local function do_race_announce(channel_id)
    fetch_cached("last_race", API_BASE .. "/last/results.json", LIVE_TTL, function(data)
        if not data then return end
        local races = data.MRData.RaceTable.Races
        if not races or #races == 0 then return end
        local race = races[1]

        -- Include the season in the key to avoid cross-season false positives.
        local key  = (data.MRData.RaceTable.season or "") .. "-" .. race.round
        local last = get_setting("last_announced_round")
        if key == last then return end  -- already announced

        -- Only announce after the race date has passed (results are final).
        if race.date > utc_date() then return end

        -- Don't announce stale results. If the race was more than ~24 hours ago
        -- (e.g. bot was offline, or this is first startup), mark it as seen and skip.
        local yesterday = os.date("!%Y-%m-%d", os.time() - 86400)
        if race.date < yesterday then
            set_setting("last_announced_round", key)
            return
        end

        local lines = { string.format("🏆 **Race Result: %s (Round %s)**", race.raceName, race.round) }
        local shown = 0
        for _, e in ipairs(race.Results or {}) do
            local pos = tonumber(e.position)
            if pos and pos <= 3 then
                lines[#lines + 1] = string.format("%s %s (%s)",
                    MEDAL[pos],
                    e.Driver.givenName .. " " .. e.Driver.familyName,
                    (e.Constructor and e.Constructor.name) or "?")
                shown = shown + 1
                if shown == 3 then break end
            end
        end
        send_message(channel_id, table.concat(lines, "\n"))
        set_setting("last_announced_round", key)
    end)
end

local function background_check()
    local channel   = get_setting("channel")
    if not channel then return end
    local reminders = get_setting("reminders")
    local announce  = get_setting("announce")
    if not reminders and not announce then return end
    if reminders then do_race_reminder(channel) end
    if announce  then do_race_announce(channel)  end
end

-- ── command router ────────────────────────────────────────────────────────────

register_command("f1", "Formula 1 season info. Run !f1 for a list of subcommands.", function(data)
    local sub  = data.args[2]
    local arg3 = data.args[3]

    if     sub == "leaderboard"                then cmd_leaderboard(data.channel_id)
    elseif sub == "constructor"                then cmd_constructor(data.channel_id)
    elseif sub == "last"                       then cmd_last(data.channel_id)
    elseif sub == "next"                       then cmd_next_race(data.channel_id)
    elseif sub == "races"                      then cmd_races(data.channel_id)
    elseif sub == "qualifying" or sub == "quali" then cmd_qualifying(data.channel_id)
    elseif sub == "podiums"                    then cmd_podiums(data.channel_id)
    elseif sub == "teammates"                  then cmd_teammates(data.channel_id)
    elseif sub == "driver" then
        local parts = {}
        for i = 3, #data.args do parts[#parts + 1] = data.args[i] end
        cmd_driver(data.channel_id, table.concat(parts, " "))
    elseif sub == "set" and arg3 == "channel"  then cmd_set_channel(data.channel_id)
    elseif sub == "reminders"                  then cmd_reminders(data.channel_id, arg3)
    elseif sub == "announce"                   then cmd_announce(data.channel_id, arg3)
    else
        send_message(data.channel_id,
            "**F1 Commands:**\n" ..
            "`!f1 leaderboard` — driver championship standings\n" ..
            "`!f1 constructor` — constructor/team standings\n" ..
            "`!f1 last` — top 10 from the most recent race\n" ..
            "`!f1 next race` — next race date & location\n" ..
            "`!f1 races` — all upcoming races\n" ..
            "`!f1 qualifying` — last qualifying grid\n" ..
            "`!f1 podiums` — podium finishers per race\n" ..
            "`!f1 driver <name>` — stats for a specific driver\n" ..
            "`!f1 teammates` — side-by-side teammate comparison\n" ..
            "`!f1 set channel` — set this channel for announcements\n" ..
            "`!f1 reminders on|off` — toggle pre-race reminders\n" ..
            "`!f1 announce on|off` — toggle automatic race results"
        )
    end
end, 5)

-- ── background timer ──────────────────────────────────────────────────────────

register_timer(CHECK_INTERVAL, background_check)
