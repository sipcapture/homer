import { useCallback, useEffect, useState } from 'react'
import { RefreshCw } from 'lucide-react'
import { apiGet } from '../api'
import { APP_BUILD_VERSION } from '@/lib/appVersion'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { BoolIndicator } from '@/components/ui/bool-indicator'
import { cn } from '@/lib/utils'
import { SettingsPageHeader } from './SettingsPageHeader'
import { SortableTableHead } from './SortableTableHead'
import { useTableSort } from './useTableSort'

export default function SystemPanel() {
  const [nodes, setNodes] = useState<any[]>([])
  const [modules, setModules] = useState<any>(null)
  const [backendVersion, setBackendVersion] = useState<string>('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const load = async () => {
    setLoading(true)
    setError('')
    try {
      const [nodesData, modulesData, health] = await Promise.all([
        apiGet('/db/nodes').catch(() => ({ data: { items: [] } })),
        apiGet('/modules').catch(() => ({ data: {} })),
        apiGet<{ version?: string }>('/health').catch(() => null),
      ])
      setNodes(nodesData?.data?.items || [])
      setModules(modulesData?.data || null)
      const v = health && typeof health.version === 'string' ? health.version.trim() : ''
      setBackendVersion(v)
    } catch (err: any) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [])

  const getSortValue = useCallback((node: any, columnKey: string) => {
    switch (columnKey) {
      case 'primary':
        return !!node.primary
      case 'online':
        return !!node.online
      default:
        return node[columnKey]
    }
  }, [])

  const { sortCol, sortDir, toggleSort, sortedItems: sortedNodes } = useTableSort(
    nodes,
    getSortValue,
  )

  return (
    <div className="space-y-6">
      <SettingsPageHeader
        title="System Overview"
        description="Database nodes and module status for the Homer backend."
        actions={
          <Button variant="outline" size="sm" onClick={load} disabled={loading}>
            <RefreshCw className={cn('mr-1.5 size-4', loading && 'animate-spin')} />
            {loading ? 'Loading...' : 'Refresh'}
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
          <CardTitle>Versions</CardTitle>
          <CardDescription>
            UI bundle semver (from build) and API backend version from{' '}
            <code className="rounded bg-muted px-1 text-[11px]">GET /api/v4/health</code>.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <dl className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div className="flex flex-col gap-0.5">
              <dt className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                UI (this bundle)
              </dt>
              <dd className="font-mono text-sm text-foreground">{APP_BUILD_VERSION}</dd>
            </div>
            <div className="flex flex-col gap-0.5">
              <dt className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                Backend (coordinator)
              </dt>
              <dd className="font-mono text-sm text-foreground">
                {backendVersion || (loading ? '…' : '—')}
              </dd>
            </div>
          </dl>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Database Nodes</CardTitle>
          <CardDescription>
            Configured storage backends and their current connection status.
          </CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          {nodes.length === 0 ? (
            <p className="p-6 text-sm text-muted-foreground">No nodes found.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <SortableTableHead
                    columnKey="name"
                    label="Name"
                    sortCol={sortCol}
                    sortDir={sortDir}
                    onSort={toggleSort}
                  />
                  <SortableTableHead
                    columnKey="host"
                    label="Host"
                    sortCol={sortCol}
                    sortDir={sortDir}
                    onSort={toggleSort}
                  />
                  <SortableTableHead
                    columnKey="node"
                    label="Node"
                    sortCol={sortCol}
                    sortDir={sortDir}
                    onSort={toggleSort}
                  />
                  <SortableTableHead
                    columnKey="primary"
                    label="Primary"
                    sortCol={sortCol}
                    sortDir={sortDir}
                    onSort={toggleSort}
                  />
                  <SortableTableHead
                    columnKey="online"
                    label="Online"
                    sortCol={sortCol}
                    sortDir={sortDir}
                    onSort={toggleSort}
                  />
                  <SortableTableHead
                    columnKey="db_name"
                    label="DB Name"
                    sortCol={sortCol}
                    sortDir={sortDir}
                    onSort={toggleSort}
                  />
                </TableRow>
              </TableHeader>
              <TableBody>
                {sortedNodes.map((node, idx) => (
                  <TableRow key={node.name || idx}>
                    <TableCell className="font-medium">{node.name || '—'}</TableCell>
                    <TableCell>{node.host || '—'}</TableCell>
                    <TableCell>{node.node || '—'}</TableCell>
                    <TableCell>
                      <BoolIndicator
                        value={!!node.primary}
                        trueLabel="Primary"
                        falseLabel="Not primary"
                      />
                    </TableCell>
                    <TableCell>
                      <BoolIndicator
                        value={!!node.online}
                        trueLabel="Online"
                        falseLabel="Offline"
                      />
                    </TableCell>
                    <TableCell>{node.db_name || '—'}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Modules Status</CardTitle>
          <CardDescription>
            Optional integrations configured on the Homer backend.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {modules ? (
            <dl className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div className="flex flex-col gap-0.5">
                <dt className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                  Loki
                </dt>
                <dd className="inline-flex items-center gap-2 text-sm text-foreground">
                  <BoolIndicator value={!!modules.loki?.enable} />
                </dd>
              </div>
              {modules.loki?.template && (
                <div className="flex flex-col gap-0.5">
                  <dt className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                    Loki Template
                  </dt>
                  <dd className="break-words text-sm text-foreground">
                    {modules.loki.template}
                  </dd>
                </div>
              )}
              {modules.loki?.external_url && (
                <div className="flex flex-col gap-0.5">
                  <dt className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                    Loki URL
                  </dt>
                  <dd className="break-words font-mono text-sm text-foreground">
                    {modules.loki.external_url}
                  </dd>
                </div>
              )}
            </dl>
          ) : (
            <p className="text-sm text-muted-foreground">
              No module information available.
            </p>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
