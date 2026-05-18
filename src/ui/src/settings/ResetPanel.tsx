import { useState } from 'react'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { HardDrive, RotateCcw, Trash2 } from 'lucide-react'
import { apiPost, apiGet } from '../api'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { SettingsPageHeader } from './SettingsPageHeader'
import { runClientResetWithConfirm, type ClientResetAction } from './clientResetActions'

export default function ResetPanel({
  readOnly = false,
  canServerReset = false,
}: {
  readOnly?: boolean
  /** Reset dashboards / mappings on the server (admin only). */
  canServerReset?: boolean
}) {
  const confirm = useConfirm()
  const [status, setStatus] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState('')

  const runReset = async (name: string, fn: () => Promise<unknown>) => {
    if (!(await confirm({ message: `Run reset action: ${name}?`, variant: 'destructive' }))) return
    setLoading(name)
    setStatus('')
    setError('')
    try {
      await fn()
      setStatus(`${name} reset completed`)
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err)
      setError(`${name} reset failed: ${msg}`)
    } finally {
      setLoading('')
    }
  }

  const runClient = async (label: string, action: ClientResetAction) => {
    setLoading(label)
    setStatus('')
    setError('')
    try {
      const r = await runClientResetWithConfirm(confirm, action)
      if (!r.ok && r.line) {
        setError(r.line)
      }
    } finally {
      setLoading('')
    }
  }

  return (
    <div className="space-y-6">
      <SettingsPageHeader
        title="Reset"
        description="Browser: UI Cache Storage, dashboard-related localStorage, full local storage, and script-visible cookies. Server-side reset (dashboards and mappings) is available to administrators only. The same browser actions are available from the dashboard header (Reset menu)."
      />

      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      {status && (
        <Alert>
          <AlertDescription>{status}</AlertDescription>
        </Alert>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <HardDrive className="size-4" />
            Client / browser
          </CardTitle>
          <CardDescription>
            <ul className="list-inside list-disc space-y-1 text-sm">
              <li>
                <strong>UI cache reset</strong> — clears Cache Storage (service worker / fetch caches) for this
                origin.
              </li>
              <li>
                <strong>Dashboard reset</strong> — removes dashboard-related keys from local storage (active
                dashboard, results columns, mini-game prefs). Does not change server data.
              </li>
              <li>
                <strong>Local storage reset</strong> — clears all keys for this origin (including session); you will
                be signed out and the page reloads.
              </li>
              <li>
                <strong>Cookie reset</strong> — best-effort removal of cookies visible to JavaScript. HttpOnly cookies
                cannot be cleared from the page; use browser settings if needed.
              </li>
            </ul>
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-wrap gap-3">
          <Button
            variant="secondary"
            onClick={() => runClient('UI cache reset', 'ui_cache')}
            disabled={!!loading}
          >
            <RotateCcw className="mr-1.5 size-4" />
            {loading === 'UI cache reset' ? 'Running...' : 'UI cache reset'}
          </Button>
          <Button
            variant="secondary"
            onClick={() => runClient('Dashboard reset', 'dashboard_local')}
            disabled={!!loading}
          >
            <RotateCcw className="mr-1.5 size-4" />
            {loading === 'Dashboard reset' ? 'Running...' : 'Dashboard reset'}
          </Button>
          <Button
            variant="destructive"
            onClick={() => runClient('Local storage reset', 'local_storage')}
            disabled={!!loading}
          >
            <Trash2 className="mr-1.5 size-4" />
            {loading === 'Local storage reset' ? 'Running...' : 'Local storage reset'}
          </Button>
          <Button
            variant="secondary"
            onClick={() => runClient('Cookie reset', 'cookies')}
            disabled={!!loading}
          >
            <RotateCcw className="mr-1.5 size-4" />
            {loading === 'Cookie reset' ? 'Running...' : 'Cookie reset'}
          </Button>
        </CardContent>
      </Card>

      {canServerReset ? (
        <Card>
          <CardHeader>
            <CardTitle>Server-side reset</CardTitle>
            <CardDescription>
              These operations affect server-side configuration and cannot be undone. Administrator
              privileges are required.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-wrap gap-3">
            <Button
              variant="destructive"
              onClick={() =>
                runReset('Dashboards', () => apiPost('/me/dashboards/reset', {}))
              }
              disabled={loading === 'Dashboards' || readOnly}
            >
              <RotateCcw className="mr-1.5 size-4" />
              {loading === 'Dashboards' ? 'Running...' : 'Reset dashboards (server)'}
            </Button>
            <Button
              variant="destructive"
              onClick={() => runReset('Mappings', () => apiGet('/mappings/reset'))}
              disabled={loading === 'Mappings' || readOnly}
            >
              <RotateCcw className="mr-1.5 size-4" />
              {loading === 'Mappings' ? 'Running...' : 'Reset mappings'}
            </Button>
          </CardContent>
        </Card>
      ) : null}
    </div>
  )
}
