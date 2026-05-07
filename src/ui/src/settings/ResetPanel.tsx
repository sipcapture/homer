import { useState } from 'react'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { RotateCcw } from 'lucide-react'
import { apiPost, apiGet } from '../api'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { SettingsPageHeader } from './SettingsPageHeader'

export default function ResetPanel({ readOnly = false }: { readOnly?: boolean }) {
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
    } catch (err) {
      setError(`${name} reset failed: ${err.message}`)
    } finally {
      setLoading('')
    }
  }

  return (
    <div className="space-y-6">
      <SettingsPageHeader
        title="Reset"
        description="Dangerous operations that reset server-side configuration to defaults."
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
          <CardTitle>Reset operations</CardTitle>
          <CardDescription>
            These operations affect server-side configuration and cannot be undone.
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
            {loading === 'Dashboards' ? 'Running...' : 'Reset Dashboards'}
          </Button>
          <Button
            variant="destructive"
            onClick={() => runReset('Mappings', () => apiGet('/mappings/reset'))}
            disabled={loading === 'Mappings' || readOnly}
          >
            <RotateCcw className="mr-1.5 size-4" />
            {loading === 'Mappings' ? 'Running...' : 'Reset Mappings'}
          </Button>
        </CardContent>
      </Card>
    </div>
  )
}
