-- ============================================================================
-- Homer call-id correlation — SIP calls (hepid=1, profile=call)
-- ============================================================================
--
-- WHAT IT DOES
--   Given the base session_ids the user clicked on in the UI, find every
--   other session_id that shares the same "correlation id" (aka x_call_id
--   / X-CID / B2B peer call-id) and return it so the transaction view can
--   merge both legs of a B2B call.
--
-- INPUTS
--   data  — table<row>  rows returned by the base SELECT on
--                       hep_proto_1_call for the requested session_ids.
--                       Each row is a Lua table with column names as
--                       keys (session_id, caller, callee, method,
--                       data_extra, ruri_user, auth_user, src_ip, dst_ip,
--                       src_port, dst_port, timestamp, ...). data_extra
--                       is a JSON string.
--   nodes — table<str>  configured data-node aliases (informational).
--   ctx   — table       request-level context:
--                         ctx.time_from   int64 (ms) user timerange start
--                         ctx.time_to     int64 (ms) user timerange end
--                         ctx.session_ids {...}      callids from request
--                         ctx.hepid       int        1 for SIP
--                         ctx.profile     string     e.g. "call"
--                         ctx.proto_type  int        mirrors hepid
--                         ctx.event_type  string     mirrors profile
--                         ctx.nodes       {...}      same as nodes arg
--
-- RETURN
--   table<str>          extra session_id values to merge into the result.
--                       Base ids are de-duplicated by the caller, so it is
--                       safe to return them too.
--
-- HELPERS REGISTERED BY THE Go ENGINE
--   executeSQL(sql)                                            -> rows
--     Run an arbitrary SELECT against the data nodes. The engine appends
--     LIMIT, validates the statement and enforces a call/row budget.
--
--   getDataByField(proto, event, field, values, from_ms, to_ms) -> rows
--     Typed helper that builds a safe
--       SELECT * FROM hep_proto_<proto>_<event>
--       WHERE <field> IN (<values>) AND timestamp BETWEEN from AND to
--     query. "field" must be [A-Za-z0-9_].
--
--   scriptLog(level, msg)    level = "debug" | "info" | "warn" | "error"
--   HashString(algo, s)      algo = "md5" | "sha1" | "sha256"
--   HashTable(op, key, val)  op   = "get" | "set" | "del"  (small KV cache)
-- ============================================================================

-- strip_b2b removes the "_b2b-NNN" suffix some B2BUAs append to Call-ID
-- so that A-leg and B-leg ids compare equal.
local function strip_b2b(id)
  if type(id) ~= "string" then return nil end
  return (id:gsub("_b2b%-%d+$", ""))
end

local function is_non_empty(s)
  return type(s) == "string" and #s > 0
end

-- add inserts s into out/seen if it is a new, non-empty string.
local function add(out, seen, s)
  if not is_non_empty(s) then return end
  if seen[s] then return end
  seen[s] = true
  out[#out + 1] = s
end

-- quote_list SQL-quotes and joins a Lua array of strings into
-- "'a','b','c'". ' characters are doubled.
local function quote_list(arr)
  local parts = {}
  for i = 1, #arr do
    parts[i] = "'" .. tostring(arr[i]):gsub("'", "''") .. "'"
  end
  return table.concat(parts, ",")
end

-- collect_correlation_ids reads the "correlation id" from a row. In
-- homer-core the historical ClickHouse column "correlation_id" lives
-- inside the data_extra JSON as x_call_id (see hep_adapter.go). We try a
-- handful of common keys so user-customised schemas keep working.
local function row_correlation_id(row)
  if type(row) ~= "table" then return nil end
  return row["correlation_id"]
      or row["x_call_id"]
      or row["xcid"]
      or row["callid_aleg"]
end

function correlate(data, nodes, ctx)
  local extras = {}
  local seen   = {}

  if type(data) ~= "table" or #data == 0 then
    return extras
  end

  -- Pull the user's timerange from ctx. It is optional: legacy scripts
  -- that ignore ctx still work, and the SQL below skips the BETWEEN
  -- clause when from/to are missing.
  local from_ms = (type(ctx) == "table" and tonumber(ctx.time_from)) or 0
  local to_ms   = (type(ctx) == "table" and tonumber(ctx.time_to))   or 0

  -- 1. Gather the two sets we want to correlate on:
  --      base_sids = session_ids the user asked for (via row.session_id)
  --      corr_ids  = correlation ids seen inside those base rows
  local base_sids = {}
  local seen_sid  = {}
  local corr_ids  = {}
  local seen_cid  = {}

  for i = 1, #data do
    local row = data[i]
    if type(row) == "table" then
      local sid = row["session_id"] or row["call_id"]
      if is_non_empty(sid) and not seen_sid[sid] then
        seen_sid[sid] = true
        base_sids[#base_sids + 1] = sid
      end

      local cid = row_correlation_id(row)
      local stripped = strip_b2b(cid)
      if is_non_empty(stripped) and not seen_cid[stripped] then
        seen_cid[stripped] = true
        corr_ids[#corr_ids + 1] = stripped
        -- A common layout is: A-leg Call-ID equals B-leg correlation_id
        -- and vice-versa. Emit the stripped id immediately so even a
        -- trivial dataset (no DB) returns something useful.
        add(extras, seen, stripped)
      end
    end
  end

  -- 2. Ask the DB for every other session_id that references any of our
  --    base session_ids OR any of the collected correlation_ids.
  --    executeSQL is the most explicit variant; getDataByField is the
  --    safer typed shortcut (see the commented-out version below).
  if #base_sids == 0 and #corr_ids == 0 then
    return extras
  end

  local filters = {}
  if #base_sids > 0 then
    -- Other dialogs whose correlation_id points at one of our sids.
    filters[#filters + 1] =
      "json_extract_string(data_extra, '$.x_call_id') IN (" ..
      quote_list(base_sids) .. ")"
  end
  if #corr_ids > 0 then
    -- Dialogs whose own session_id is one of our correlation_ids.
    filters[#filters + 1] = "session_id IN (" .. quote_list(corr_ids) .. ")"
    -- Or dialogs that share the same correlation_id (chained B2BUAs).
    filters[#filters + 1] =
      "json_extract_string(data_extra, '$.x_call_id') IN (" ..
      quote_list(corr_ids) .. ")"
  end

  local sql = "SELECT DISTINCT session_id FROM homer_lake.hep_proto_1_call" ..
              " WHERE (" .. table.concat(filters, " OR ") .. ")"

  -- Bound the search to the user's visible timerange when available.
  -- Without this bound a busy DuckLake would scan every historical
  -- partition, which is almost never what the UI wants.
  if from_ms > 0 and to_ms > 0 and from_ms < to_ms then
    sql = sql ..
      " AND timestamp >= (to_timestamp(" .. tostring(from_ms) .. " / 1000.0) AT TIME ZONE 'UTC')" ..
      " AND timestamp <= (to_timestamp(" .. tostring(to_ms)   .. " / 1000.0) AT TIME ZONE 'UTC')"
  end

  local rows = executeSQL(sql)
  if rows == nil or #rows == 0 then
    return extras
  end

  for i = 1, #rows do
    add(extras, seen, rows[i].session_id)
  end

  -- -------------------------------------------------------------------------
  -- ALTERNATIVE 1: use the typed helper (safer, no hand-written SQL).
  --   -- from_ms/to_ms are already read from ctx at the top of the script.
  --   local rows = getDataByField(1, "call", "session_id", corr_ids, from_ms, to_ms)
  --   for i = 1, #(rows or {}) do
  --     add(extras, seen, rows[i].session_id)
  --   end
  --
  -- ALTERNATIVE 2: correlate by a top-level column if your deployment
  -- maps X-CID to a real column named "correlation_id":
  --   local sql = "SELECT DISTINCT session_id FROM homer_lake.hep_proto_1_call" ..
  --               " WHERE correlation_id IN (" .. quote_list(base_sids) .. ")"
  --   local rows = executeSQL(sql)
  -- -------------------------------------------------------------------------

  return extras
end
