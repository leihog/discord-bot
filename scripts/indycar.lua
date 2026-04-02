-- IndyCar commands
-- Data from TheSportsDB (https://www.thesportsdb.com/api/v1/json/123/)
--
-- Commands:
--   !indy races          — upcoming race schedule for the season
--   !indy last           — top 10 results from the most recent race
--   !indy next           — date, time, and location of the next race
--   !indy leaderboard    — driver championship standings
--   !indy set channel    — set this channel for announcements & reminders
--   !indy reminders on|off — toggle pre-race reminder (~1 hour before start)
--   !indy announce on|off  — toggle automatic podium announcement after each race

local CACHE_TTL   = 900    -- 15 minutes (all data from single season endpoint)
local API_BASE    = "https://www.thesportsdb.com/api/v1/json/123"
local LEAGUE_ID   = "4373"
local CACHE_NS    = "indy_cache"
local SETTINGS_NS = "indy_settings"
local CHECK_FAST  =   300  --  5 min: race within ~2 hours, or waiting for results
local CHECK_NEAR  =  1800  -- 30 min: race day, not imminent
local CHECK_SOON  =  7200  --  2 h:   race tomorrow
local CHECK_IDLE  = 21600  --  6 h:   race more than 2 days away

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

-- "YYYY-MM-DDTHH:MM:SS" or "YYYY-MM-DD HH:MM:SS" → "HH:MM UTC"
local function fmt_timestamp(ts)
    local hm = ts and ts:match("%d%d%d%d%-%d%d%-%d%d[T ](%d%d:%d%d)")
    return hm and (hm .. " UTC") or (ts or "?")
end

-- Current date in UTC as "YYYY-MM-DD"
local function utc_date()
    return os.date("!%Y-%m-%d")
end

-- Current UTC time as minutes since midnight
local function utc_minutes_now()
    return tonumber(os.date("!%H")) * 60 + tonumber(os.date("!%M"))
end

-- "YYYY-MM-DDTHH:MM:SS" or "YYYY-MM-DD HH:MM:SS" → minutes since midnight
local function timestamp_to_minutes(ts)
    local h, m = ts and ts:match("%d%d%d%d%-%d%d%-%d%d[T ](%d%d):(%d%d)")
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

-- ── strResult parsers ─────────────────────────────────────────────────────────

-- strResult comes in three variants (see race_result.md), but all share:
--   • A tab-delimited race results section as the first block
--   • An optional standings section introduced by "Top 10 Driver Standings"
--   • Sections separated by a line of dashes ("---...")
-- Field values are prefixed with "/" which we strip during parsing.

local function normalize(s)
    return s:gsub("\r\n", "\n"):gsub("\r", "\n")
end

-- Extract trimmed fields from one line, stripping leading "/" from each value.
-- Tab-delimited:   "1 \t/Alex Palou \t\t/Chip Ganassi Racing \t\t/01:52:21.6997 \t"
-- Space-delimited: "1 /Kyle Kirkwood /Andretti Global w/ Curb-Agajanian /126"
-- The space-delimited case splits on " /" (space+slash). Team names like
-- "Andretti Global w/ Curb-Agajanian" are safe because "w/" has no space before "/".
local function split_fields(line)
    local fields = {}
    if line:find("\t") then
        for f in line:gmatch("[^\t]+") do
            local v = f:match("^%s*(.-)%s*$")
            if v ~= "" then fields[#fields + 1] = v:gsub("^/", "") end
        end
    else
        local s = line:gsub(" /", "\0")
        for part in (s .. "\0"):gmatch("([^\0]*)\0") do
            local v = part:match("^%s*(.-)%s*$")
            if v ~= "" then fields[#fields + 1] = v end
        end
    end
    return fields
end

-- Parse the race results section of strResult.
-- Returns array of { pos, driver, team, time }.
local function parse_race_results(str_result)
    if not str_result or str_result == "" then return {} end
    local s = normalize(str_result)
    -- Take only the first section (before any "---" separator line)
    local section = s:match("^(.-)\n%-%-%-+") or s
    local results = {}
    for line in section:gmatch("[^\n]+") do
        if line:match("%S") then
            local f   = split_fields(line)
            local pos = f[1] and tonumber(f[1])
            if pos and f[2] then
                results[#results + 1] = {
                    pos    = pos,
                    driver = f[2],
                    team   = f[3] or "?",
                    time   = f[4] or "?",
                }
            end
        end
    end
    table.sort(results, function(a, b) return a.pos < b.pos end)
    return results
end

-- Parse the standings section of strResult if present.
-- Returns array of { pos, driver, team, points }, or nil if no standings found.
local function parse_standings(str_result)
    if not str_result or str_result == "" then return nil end
    local s = normalize(str_result)
    local after = s:match("Top 10 Driver Standings[^\n]*\n(.*)")
    if not after then return nil end
    after = after:match("Pos /Driver /Team /Points[^\n]*\n(.*)")
    if not after then return nil end
    local standings = {}
    for line in after:gmatch("[^\n]+") do
        if line:match("%S") and not line:match("^%-%-%-") then
            local f   = split_fields(line)
            local pos = f[1] and tonumber(f[1])
            local pts = f[4] and tonumber(f[4])
            if pos and f[2] then
                standings[#standings + 1] = {
                    pos    = pos,
                    driver = f[2],
                    team   = f[3] or "?",
                    points = pts or 0,
                }
            end
        end
    end
    if #standings == 0 then return nil end
    table.sort(standings, function(a, b) return a.pos < b.pos end)
    return standings
end

-- ── season data helpers ───────────────────────────────────────────────────────

local function season_url(year)
    return API_BASE .. "/eventsseason.php?id=" .. LEAGUE_ID .. "&s=" .. tostring(year)
end

-- Fetch the season event list for the most relevant year.
-- Handles December (try next year first) and January (fall back to prior year).
-- Calls callback(events, year) or callback(nil, err).
local function fetch_season(callback)
    local year  = tonumber(os.date("!%Y"))
    local month = tonumber(os.date("!%m"))

    local function try_year(y, fallback)
        local key = "season_" .. y
        fetch_cached(key, season_url(y), CACHE_TTL, function(data, err)
            if not data then
                if fallback then fallback() else callback(nil, err) end
                return
            end
            local events = data.events or {}
            if #events == 0 and fallback then
                fallback()
            else
                callback(events, y)
            end
        end)
    end

    if month == 12 then
        try_year(year + 1, function() try_year(year, nil) end)
    elseif month == 1 then
        try_year(year, function() try_year(year - 1, nil) end)
    else
        try_year(year, nil)
    end
end

-- Returns true if an event is considered finished.
local function is_finished(ev)
    return (ev.strStatus == "Match Finished") or
           (ev.strResult and ev.strResult ~= "")
end

-- Most recently completed race (latest dateEvent <= today that is finished).
local function find_last_race(events)
    local today = utc_date()
    local last  = nil
    for _, ev in ipairs(events) do
        if ev.dateEvent and ev.dateEvent <= today and is_finished(ev) then
            if not last or ev.dateEvent > last.dateEvent or
               (ev.dateEvent == last.dateEvent and
                tonumber(ev.intRound or "0") > tonumber(last.intRound or "0")) then
                last = ev
            end
        end
    end
    return last
end

-- Next upcoming race (earliest dateEvent >= today that is not yet finished).
local function find_next_race(events)
    local today   = utc_date()
    local next_ev = nil
    for _, ev in ipairs(events) do
        if ev.dateEvent and ev.dateEvent >= today and not is_finished(ev) then
            if not next_ev or ev.dateEvent < next_ev.dateEvent then
                next_ev = ev
            end
        end
    end
    return next_ev
end

-- Scan completed events newest-first; return standings from the first event
-- whose strResult is a standings block.
local function find_best_standings(events)
    local today     = utc_date()
    local completed = {}
    for _, ev in ipairs(events) do
        if ev.dateEvent and ev.dateEvent <= today and is_finished(ev) then
            completed[#completed + 1] = ev
        end
    end
    table.sort(completed, function(a, b) return a.dateEvent > b.dateEvent end)
    for _, ev in ipairs(completed) do
        local standings = parse_standings(ev.strResult)
        if standings then return standings, ev end
    end
    return nil, nil
end

-- ── commands ──────────────────────────────────────────────────────────────────

local function cmd_races(channel_id)
    fetch_season(function(events, year)
        if not events then
            send_message(channel_id, "Could not fetch IndyCar schedule.")
            return
        end
        local today    = utc_date()
        local upcoming = {}
        for _, ev in ipairs(events) do
            if ev.dateEvent and ev.dateEvent >= today and not is_finished(ev) then
                upcoming[#upcoming + 1] = ev
            end
        end
        table.sort(upcoming, function(a, b) return a.dateEvent < b.dateEvent end)
        if #upcoming == 0 then
            send_message(channel_id, string.format(
                "No upcoming races — the %s IndyCar season is over. Stay tuned for next year!", year))
            return
        end
        local lines = { string.format("**Upcoming %s IndyCar Races** (%d remaining)", year, #upcoming) }
        for _, ev in ipairs(upcoming) do
            local venue = ev.strVenue or ev.strCity or "?"
            lines[#lines + 1] = string.format("Rd %s — %s — %s (%s)",
                ev.intRound or "?", ev.strEvent or "?", ev.dateEvent, venue)
        end
        send_lines(channel_id, lines)
    end)
end

local function cmd_last(channel_id)
    fetch_season(function(events)
        if not events then
            send_message(channel_id, "Could not fetch race data.")
            return
        end
        local race = find_last_race(events)
        if not race then
            send_message(channel_id, "No completed races found yet this season.")
            return
        end
        local results = parse_race_results(race.strResult)
        if #results == 0 then
            send_message(channel_id, string.format(
                "**%s** — no detailed results available yet.", race.strEvent or "?"))
            return
        end
        local lines = { string.format("**%s — Round %s (%s)**",
            race.strEvent or "?", race.intRound or "?", race.dateEvent) }
        for i, r in ipairs(results) do
            if i > 10 then break end
            local badge = MEDAL[r.pos] or string.format("%2d.", r.pos)
            lines[#lines + 1] = string.format("%s %s (%s) — %s", badge, r.driver, r.team, r.time)
        end
        send_message(channel_id, table.concat(lines, "\n"))
    end)
end

local function cmd_next(channel_id)
    fetch_season(function(events, year)
        if not events then
            send_message(channel_id, "Could not fetch race data.")
            return
        end
        local race = find_next_race(events)
        if not race then
            send_message(channel_id, string.format(
                "No upcoming races — the %s season may be over. Stay tuned for next year!", year))
            return
        end
        local time_str = ""
        if race.strTimestamp and race.strTimestamp ~= "" then
            local t = fmt_timestamp(race.strTimestamp)
            if t ~= "00:00 UTC" then time_str = " at " .. t end
        end
        local parts = {}
        if race.strVenue and race.strVenue ~= "" then parts[#parts + 1] = race.strVenue end
        if race.strCity   and race.strCity   ~= "" then parts[#parts + 1] = race.strCity end
        if race.strCountry and race.strCountry ~= "" then parts[#parts + 1] = race.strCountry end
        local location = #parts > 0 and table.concat(parts, ", ") or "?"
        send_message(channel_id, string.format(
            "**Next Race: %s (Round %s)**\n📅 %s%s\n🏎️  %s",
            race.strEvent or "?", race.intRound or "?",
            race.dateEvent, time_str, location
        ))
    end)
end

local function cmd_leaderboard(channel_id)
    fetch_season(function(events, year)
        if not events then
            send_message(channel_id, "Could not fetch standings data.")
            return
        end
        local standings, source_race = find_best_standings(events)
        if not standings or #standings == 0 then
            send_message(channel_id,
                "No championship standings available yet — check back after the first race!")
            return
        end
        local after_str = ""
        if source_race then
            after_str = " (after " .. (source_race.strEvent or source_race.dateEvent) .. ")"
        end
        local lines = { string.format("**%s IndyCar Driver Standings**%s", year, after_str) }
        for i, s in ipairs(standings) do
            if i > 10 then break end
            local badge = MEDAL[s.pos] or string.format("%2d.", s.pos)
            lines[#lines + 1] = string.format("%s %s (%s) — %s pts", badge, s.driver, s.team, s.points)
        end
        send_message(channel_id, table.concat(lines, "\n"))
    end)
end

-- ── settings commands ─────────────────────────────────────────────────────────

local function cmd_set_channel(channel_id)
    set_setting("channel", channel_id)
    send_message(channel_id, "✅ This channel will receive IndyCar race announcements and reminders (when enabled).\n" ..
        "Use `!indy reminders on` and `!indy announce on` to activate them.")
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
            ". Use `!indy reminders on` or `!indy reminders off` to change.")
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
            ". Use `!indy announce on` or `!indy announce off` to change.")
    end
end

-- ── background checks ─────────────────────────────────────────────────────────

local function do_race_reminder(channel_id)
    fetch_season(function(events)
        if not events then return end
        local race = find_next_race(events)
        if not race or not race.strTimestamp or race.strTimestamp == "" then return end

        local last = get_setting("last_reminder_round")
        if race.intRound == last then return end  -- already reminded for this race

        if race.dateEvent ~= utc_date() then return end  -- not today

        local race_min = timestamp_to_minutes(race.strTimestamp)
        local now_min  = utc_minutes_now()
        if not race_min or race_min == 0 then return end  -- 00:00 is a placeholder, not a real time
        local diff = race_min - now_min

        if diff >= 50 and diff <= 70 then
            local parts = {}
            if race.strVenue  and race.strVenue  ~= "" then parts[#parts+1] = race.strVenue  end
            if race.strCity   and race.strCity   ~= "" then parts[#parts+1] = race.strCity   end
            if race.strCountry and race.strCountry ~= "" then parts[#parts+1] = race.strCountry end
            send_message(channel_id, string.format(
                "🏁 **Race starting in ~1 hour!**\n%s (Round %s) — %s\nStarts at %s",
                race.strEvent or "?", race.intRound or "?",
                table.concat(parts, ", "), fmt_timestamp(race.strTimestamp)
            ))
            set_setting("last_reminder_round", race.intRound)
        end
    end)
end

local function do_race_announce(channel_id)
    fetch_season(function(events, year)
        if not events then return end
        local race = find_last_race(events)
        if not race then return end

        local key  = tostring(year) .. "-" .. (race.intRound or race.dateEvent)
        local last = get_setting("last_announced_round")
        if key == last then return end  -- already announced

        if race.dateEvent > utc_date() then return end  -- safety guard

        -- Skip stale results (bot was offline for > ~24 h): mark seen, don't announce.
        local yesterday = os.date("!%Y-%m-%d", os.time() - 86400)
        if race.dateEvent < yesterday then
            set_setting("last_announced_round", key)
            return
        end

        local results = parse_race_results(race.strResult)
        if #results == 0 then return end  -- no results yet, retry next tick

        local lines = { string.format("🏆 **Race Result: %s (Round %s)**",
            race.strEvent or "?", race.intRound or "?") }
        for _, r in ipairs(results) do
            if r.pos > 3 then break end
            lines[#lines + 1] = string.format("%s %s (%s)", MEDAL[r.pos] or tostring(r.pos), r.driver, r.team)
        end
        send_message(channel_id, table.concat(lines, "\n"))
        set_setting("last_announced_round", key)
    end)
end

-- Determine next poll interval by reading the already-cached season data (no HTTP).
local function compute_interval()
    local year  = tonumber(os.date("!%Y"))
    local month = tonumber(os.date("!%m"))
    local try_years = { year }
    if month == 12 then try_years = { year + 1, year } end
    if month == 1  then try_years = { year, year - 1 } end

    for _, y in ipairs(try_years) do
        local cached = store_get(CACHE_NS, "season_" .. y)
        if cached and cached.data then
            local events = cached.data.events or {}
            local race   = find_next_race(events)
            if race then
                local today = utc_date()
                if race.dateEvent < today then
                    return CHECK_FAST  -- past race date, polling for results
                elseif race.dateEvent == today then
                    if race.strTimestamp and race.strTimestamp ~= "" then
                        local diff = timestamp_to_minutes(race.strTimestamp) - utc_minutes_now()
                        if diff and diff <= 120 then return CHECK_FAST end
                    end
                    return CHECK_NEAR
                elseif race.dateEvent <= os.date("!%Y-%m-%d", os.time() + 86400) then
                    return CHECK_SOON
                else
                    return CHECK_IDLE
                end
            end
        end
    end
    return CHECK_NEAR
end

local function background_check()
    local channel  = get_setting("channel")
    if not channel then return end
    local reminders = get_setting("reminders")
    local announce  = get_setting("announce")
    if not reminders and not announce then return end
    if reminders then do_race_reminder(channel) end
    if announce  then do_race_announce(channel)  end
end

-- ── command router ────────────────────────────────────────────────────────────

register_command("indy", "IndyCar season info. Run !indy for a list of subcommands.", function(data)
    local sub  = data.args[2]
    local arg3 = data.args[3]

    if     sub == "races"                      then cmd_races(data.channel_id)
    elseif sub == "last"                       then cmd_last(data.channel_id)
    elseif sub == "next"                       then cmd_next(data.channel_id)
    elseif sub == "leaderboard"                then cmd_leaderboard(data.channel_id)
    elseif sub == "set" and arg3 == "channel"  then cmd_set_channel(data.channel_id)
    elseif sub == "reminders"                  then cmd_reminders(data.channel_id, arg3)
    elseif sub == "announce"                   then cmd_announce(data.channel_id, arg3)
    else
        send_message(data.channel_id,
            "**IndyCar Commands:**\n" ..
            "`!indy races` — upcoming race schedule\n" ..
            "`!indy last` — top 10 from the most recent race\n" ..
            "`!indy next` — next race date, time & location\n" ..
            "`!indy leaderboard` — driver championship standings\n" ..
            "`!indy set channel` — set this channel for announcements\n" ..
            "`!indy reminders on|off` — toggle pre-race reminders\n" ..
            "`!indy announce on|off` — toggle automatic race results"
        )
    end
end, 5)

-- ── background timer ──────────────────────────────────────────────────────────

-- Self-rescheduling check: after each run, compute how long to wait based on
-- how far away the next race is, then schedule the next run via call_later.
local run_background_check
run_background_check = function()
    background_check()
    call_later(compute_interval(), run_background_check)
end

call_later(CHECK_FAST, run_background_check)
