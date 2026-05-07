import { useState } from 'react'
import { Download } from 'lucide-react'
import { apiGet, apiBase } from '../api'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { ScrollArea } from '@/components/ui/scroll-area'
import { SettingsPageHeader } from './SettingsPageHeader'

export default function ApiDocsPanel() {
  const [spec, setSpec] = useState<any>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const load = async () => {
    setLoading(true)
    setError('')
    try {
      const data = await apiGet('/openapi.json')
      setSpec(data || null)
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="space-y-6">
      <SettingsPageHeader
        title="API Documentation"
        description="Homer exposes a JSON:API-compatible REST interface for all resources."
        actions={
          <Button variant="outline" size="sm" onClick={load} disabled={loading}>
            <Download className="mr-1.5 size-4" />
            {loading ? 'Loading...' : 'Load OpenAPI'}
          </Button>
        }
      />

      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <Card>
        <CardHeader>
          <CardTitle>Endpoints</CardTitle>
          <CardDescription>Base URLs for accessing the API.</CardDescription>
        </CardHeader>
        <CardContent>
          <dl className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div className="flex flex-col gap-1">
              <dt className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                Base URL
              </dt>
              <dd className="font-mono text-sm text-foreground">{apiBase}</dd>
            </div>
            <div className="flex flex-col gap-1">
              <dt className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                OpenAPI URL
              </dt>
              <dd className="font-mono text-sm text-foreground">
                {apiBase}/openapi.json
              </dd>
            </div>
          </dl>
        </CardContent>
      </Card>

      {spec ? (
        <Card>
          <CardHeader>
            <CardTitle>{spec.info?.title || 'OpenAPI Specification'}</CardTitle>
            <CardDescription>
              Version {spec.info?.version || '—'} · {Object.keys(spec.paths || {}).length}{' '}
              paths
            </CardDescription>
          </CardHeader>
          <CardContent>
            <ScrollArea className="h-[420px]">
              <pre className="whitespace-pre-wrap bg-muted/40 p-3 font-mono text-[11px]">
                {JSON.stringify(spec, null, 2)}
              </pre>
            </ScrollArea>
          </CardContent>
        </Card>
      ) : (
        <Card>
          <CardContent className="py-8 text-center">
            <p className="text-sm text-muted-foreground">
              Click "Load OpenAPI" to fetch the API specification.
            </p>
          </CardContent>
        </Card>
      )}
    </div>
  )
}
