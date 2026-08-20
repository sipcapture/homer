import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { UI_CONTENT_SECURITY_POLICY } from './csp'

const here = dirname(fileURLToPath(import.meta.url))

describe('UI CSP', () => {
  it('does not allow script unsafe-eval or unsafe-inline', () => {
    const scriptDir = UI_CONTENT_SECURITY_POLICY.split(';')
      .map((d) => d.trim())
      .find((d) => d.startsWith('script-src'))
    expect(scriptDir).toBe("script-src 'self'")
  })

  it('matches the coordinator Go constant', () => {
    const go = readFileSync(join(here, '../../../coordinator/security_headers.go'), 'utf8')
    expect(go).toContain(UI_CONTENT_SECURITY_POLICY)
  })
})
