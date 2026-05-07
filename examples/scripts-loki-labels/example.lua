-- Custom Loki labels (same pattern as heplify-server PR #595):
-- 1) remote_logging.loki.loki_custom_labels must include each key you use.
-- 2) Max 5 distinct label keys per HEP message; invalid keys/values are skipped (debug log).
-- Example: set a static tenant for demo (replace with your logic).
function example_loki_labels()
  if GetHEPProtoType() ~= 1 then
    return
  end
  local raw = GetRawMessage()
  if raw == nil or raw == "" then
    return
  end
  -- SetLokiLabel only works if "tenant" is listed under remote_logging.loki.loki_custom_labels
  SetLokiLabel("tenant", "demo")
end
