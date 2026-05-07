/**
 * Semver string injected at `vite build` from `src/version.go` (same source as
 * the Go binary when UI is built with the release pipeline). In `vite dev`
 * without define, falls back to `dev`.
 */
export const APP_BUILD_VERSION: string =
  typeof __HOMER_APP_VERSION__ !== 'undefined' ? __HOMER_APP_VERSION__ : 'dev'
