/** Production UI CSP. Keep in sync with coordinator.UIContentSecurityPolicy. */
export const UI_CONTENT_SECURITY_POLICY =
  "default-src 'self'; base-uri 'self'; object-src 'none'; frame-ancestors 'self'; form-action 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self' ws: wss:; worker-src 'self' blob:; child-src 'self' blob:; frame-src 'self' http: https:; media-src 'self' blob:; wasm-src 'self'"
