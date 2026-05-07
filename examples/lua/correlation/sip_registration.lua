-- ============================================================================
-- Homer registration correlation — SIP REGISTER (hepid=1, profile=registration)
-- ============================================================================
--
-- Given one REGISTER session_id, return every other REGISTER session_id
-- for the same Address-of-Record (aor) inside the user's time window.
-- Useful to see the full authentication handshake (401/407/200) and
-- nearby re-registrations as a single timeline.
--
-- See the hep_proto_1_call template for a full list of available
-- helpers (executeSQL, getDataByField, scriptLog, HashString, ...).
-- ============================================================================

local function is_non_empty(s)
  return type(s) == "string" and #s > 0
end

local function add(out, seen, s)
  if not is_non_empty(s) then return end
  if seen[s] then return end
  seen[s] = true
  out[#out + 1] = s
end

function correlate(data, nodes, ctx)
  local extras = {}
  local seen   = {}

  if type(data) ~= "table" or #data == 0 then
    return extras
  end

  -- Carry the user's visible timerange into the helper so the SELECT is
  -- bounded to the partitions the UI is actually looking at.
  local from_ms = (type(ctx) == "table" and tonumber(ctx.time_from)) or 0
  local to_ms   = (type(ctx) == "table" and tonumber(ctx.time_to))   or 0

  -- Collect distinct AORs from the base rows. We fall back to a few
  -- common identifiers so custom mappings keep working.
  local aors = {}
  local seen_aor = {}
  for i = 1, #data do
    local row = data[i]
    if type(row) == "table" then
      local a = row["aor"] or row["callee"] or row["ruri_user"] or row["auth_user"]
      if is_non_empty(a) and not seen_aor[a] then
        seen_aor[a] = true
        aors[#aors + 1] = a
      end
    end
  end

  if #aors == 0 then
    return extras
  end

  -- getDataByField builds a safe SELECT *, enforces LIMIT and parameter
  -- quoting. We pass the ctx timerange so the resulting SELECT is
  -- partition-bounded instead of scanning the full lake; from_ms/to_ms
  -- default to 0 when the caller didn't set Timestamp.From/To, and the
  -- helper treats that as "no timestamp filter".
  local rows = getDataByField(1, "registration", "aor", aors, from_ms, to_ms)
  if rows == nil or #rows == 0 then
    return extras
  end

  for i = 1, #rows do
    add(extras, seen, rows[i].session_id)
  end

  return extras
end
