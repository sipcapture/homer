-- ============================================================================
-- Homer call-id correlation — SIP calls (hepid=1, profile=call)
-- ============================================================================
--
-- WHAT IT DOES
--   Given the base session_ids the user clicked on in the UI, find every
--   other session_id that shares the same "correlation id" (aka x_call_id
--   / X-CID / B2B peer call-id) and return it so the transaction view can
--   merge every leg of a B2B call.
--
--   Expansion is TRANSITIVE. Chained B2BUAs produce a chain of legs, where
--   each leg's x_call_id names its parent:
--
--       leg_a <- leg_b <- leg_c
--             <- leg_d <- leg_e
--
--   A single expansion pass only reaches the immediate neighbours of the
--   rows it was handed, so the set of legs returned would depend on which
--   leg the user happened to open (opening leg_a finds 4 of the 5 above,
--   opening leg_e finds 2). Expanding to a fixed point means every leg
--   resolves the whole call, no matter where the user entered it.
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

-- Hard cap on expansion rounds. Each round costs exactly one executeSQL
-- call, so this stays well inside the engine's MaxSQLCalls budget (default
-- 16). Chains deeper than this degrade to a partial merge, never an error.
local MAX_ROUNDS = 5

-- strip_b2b removes the "_b2b-NNN" suffix some B2BUAs append to Call-ID
-- so that A-leg and B-leg ids compare equal.
local function strip_b2b(id)
  if type(id) ~= "string" then return nil end
  return (id:gsub("_b2b%-%d+$", ""))
end

local function is_non_empty(s)
  return type(s) == "string" and #s > 0
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

-- row_correlation_id reads the "correlation id" from a row. In homer-core
-- the historical ClickHouse column "correlation_id" lives inside the
-- data_extra JSON as x_call_id (see hep_adapter.go). We try a handful of
-- common keys so user-customised schemas keep working.
local function row_correlation_id(row)
  if type(row) ~= "table" then return nil end
  local v = row["correlation_id"]
      or row["x_call_id"]
      or row["xcid"]
      or row["callid_aleg"]
  if is_non_empty(v) then return v end

  -- The default schema stores x_call_id inside the data_extra JSON
  -- string, not as a top-level column, so extract it from there. This is
  -- what makes B-leg -> A-leg correlation work: the B-leg row carries
  -- x_call_id = A-leg Call-ID.
  local de = row["data_extra"]
  if type(de) == "string" then
    v = de:match('"x_call_id"%s*:%s*"([^"]*)"')
    if is_non_empty(v) then return v end
  end

  -- HEP cid column: the decoder stores the aleg id there when the
  -- capture agent did not send its own correlation chunk. When unused it
  -- just mirrors session_id, which we must not treat as a peer id.
  local cid = row["cid"]
  if is_non_empty(cid) and cid ~= row["session_id"] then return cid end

  return nil
end

function correlate(data, nodes, ctx)
  local extras = {}
  local seen   = {}   -- every id already emitted, so extras stay unique

  if type(data) ~= "table" or #data == 0 then
    return extras
  end

  -- Pull the user's timerange from ctx. It is optional: legacy scripts
  -- that ignore ctx still work, and the SQL below skips the BETWEEN
  -- clause when from/to are missing.
  local from_ms = (type(ctx) == "table" and tonumber(ctx.time_from)) or 0
  local to_ms   = (type(ctx) == "table" and tonumber(ctx.time_to))   or 0

  -- known    = every id discovered so far.
  -- frontier = the ids still to be expanded in the next round.
  --
  -- session_ids and correlation ids share one namespace: in a B2B chain a
  -- leg's x_call_id IS another leg's session_id. So both go into the same
  -- set and the graph is walked undirected, which is what lets any leg
  -- reach the whole call.
  local known    = {}
  local frontier = {}

  local function discover(id)
    local s = strip_b2b(id)
    if not is_non_empty(s) then return end
    if known[s] then return end
    known[s] = true
    frontier[#frontier + 1] = s
    if not seen[s] then
      seen[s] = true
      extras[#extras + 1] = s
    end
  end

  -- Round 0: seed from the rows the base SELECT already returned. Emitting
  -- the correlation ids immediately means even a trivial dataset (no DB)
  -- returns something useful.
  for i = 1, #data do
    local row = data[i]
    if type(row) == "table" then
      discover(row["session_id"] or row["call_id"])
      discover(row_correlation_id(row))
    end
  end

  if #frontier == 0 then
    return extras
  end

  -- Bound the search to the user's visible timerange when available.
  -- Without this bound a busy DuckLake would scan every historical
  -- partition, which is almost never what the UI wants.
  local time_clause = ""
  if from_ms > 0 and to_ms > 0 and from_ms < to_ms then
    time_clause =
      " AND timestamp >= (to_timestamp(" .. tostring(from_ms) .. " / 1000.0) AT TIME ZONE 'UTC')" ..
      " AND timestamp <= (to_timestamp(" .. tostring(to_ms)   .. " / 1000.0) AT TIME ZONE 'UTC')"
  end

  -- Walk outwards until a round adds nothing new (fixed point) or the
  -- round cap is reached.
  for _ = 1, MAX_ROUNDS do
    local ids = quote_list(frontier)
    frontier = {}

    -- Both directions of the relation in one query:
    --   session_id IN (...)  -> legs whose own Call-ID we already know of
    --   x_call_id  IN (...)  -> legs pointing AT something we know
    --
    -- x_call_id is selected as well as filtered on: that is what makes
    -- iteration possible, since a newly found leg's parent has to be known
    -- before the next round can expand it.
    local sql =
      "SELECT DISTINCT session_id," ..
      " json_extract_string(data_extra, '$.x_call_id') AS x_call_id" ..
      " FROM homer_lake.hep_proto_1_call" ..
      " WHERE (session_id IN (" .. ids .. ")" ..
      " OR json_extract_string(data_extra, '$.x_call_id') IN (" .. ids .. "))" ..
      time_clause

    local rows = executeSQL(sql)
    if rows == nil or #rows == 0 then
      break
    end

    for i = 1, #rows do
      discover(rows[i].session_id)
      discover(rows[i].x_call_id)
    end

    if #frontier == 0 then
      break
    end
  end

  -- -------------------------------------------------------------------------
  -- ALTERNATIVE: correlate by a top-level column if your deployment maps
  -- X-CID to a real column named "correlation_id":
  --   local sql = "SELECT DISTINCT session_id, correlation_id" ..
  --               " FROM homer_lake.hep_proto_1_call" ..
  --               " WHERE correlation_id IN (" .. ids .. ")" ..
  --               " OR session_id IN (" .. ids .. ")"
  -- -------------------------------------------------------------------------

  return extras
end
