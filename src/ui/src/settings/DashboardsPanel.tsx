import { useEffect, useMemo, useState } from 'react'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { Plus, RefreshCw, RotateCcw } from 'lucide-react'
import { EditIconButton, DeleteIconButton } from '@/components/ui/table-action-buttons'
import { apiGet, apiPost, apiPut, apiDelete } from '../api'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { BoolIndicator } from '@/components/ui/bool-indicator'
import { DigitsInput, parseUint } from '@/components/ui/digits-input'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { cn } from '@/lib/utils'
import { CrudEditDialog, CrudModal, Field, settingsEditTitle } from './CrudTable'
import { SettingsPageHeader } from './SettingsPageHeader'
import { sortDashboardsByWeight } from '../dashboard/utils/dashboard-sort'

const dashboardTypeLabel = (type: number) => {
  switch (type) {
    case 1:
      return 'Normal'
    case 2:
      return 'Iframe'
    case 4:
      return 'Search'
    case 6:
      return 'Tab'
    case 7:
      return 'Grafana'
    default:
      return `Type ${type}`
  }
}

export default function DashboardsPanel({ readOnly = false }: { readOnly?: boolean }) {
  const confirm = useConfirm()
  const [dashboards, setDashboards] = useState<any[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')
  const [createOpen, setCreateOpen] = useState(false)
  const [editingDashboard, setEditingDashboard] = useState<any>(null)
  const [form, setForm] = useState({
    id: '',
    name: '',
    type: 1,
    shared: false,
    weight: '10',
  })
  const [filters, setFilters] = useState({
    search: '',
    type: '',
    shared: '',
  })

  const load = async () => {
    setLoading(true)
    setError('')
    try {
      const data = await apiGet('/dashboards')
      setDashboards(sortDashboardsByWeight(data?.data?.items || []))
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [])

  const filteredDashboards = useMemo(() => {
    let list = dashboards
    const q = filters.search.trim().toLowerCase()
    if (q) {
      list = list.filter((d) => {
        const name = String(d.name ?? '').toLowerCase()
        const id = String(d.id ?? d.param ?? '').toLowerCase()
        return name.includes(q) || id.includes(q)
      })
    }
    if (filters.type !== '') {
      const t = Number(filters.type)
      list = list.filter((d) => Number(d.type) === t)
    }
    if (filters.shared === 'true') {
      list = list.filter((d) => d.shared === true)
    }
    if (filters.shared === 'false') {
      list = list.filter((d) => !d.shared)
    }
    return list
  }, [dashboards, filters])

  const resetForm = () => {
    setEditingDashboard(null)
    setCreateOpen(false)
    setForm({ id: '', name: '', type: 1, shared: false, weight: '10' })
  }

  const startEdit = (dashboard) => {
    setEditingDashboard(dashboard)
    setForm({
      id: dashboard.id || '',
      name: dashboard.name || '',
      type: dashboard.type || 1,
      shared: dashboard.shared || false,
      weight: dashboard.weight != null ? String(dashboard.weight) : '10',
    })
  }

  const save = async () => {
    setError('')
    setSuccess('')
    try {
      const payload = {
        id: form.id,
        name: form.name,
        param: form.id,
        type: Number(form.type),
        shared: form.shared,
        weight: parseUint(form.weight, 10),
      }
      if (editingDashboard) {
        await apiPut(`/dashboards/${editingDashboard.id}`, payload)
        setSuccess('Dashboard updated')
      } else {
        if (!form.id || !form.name) {
          setError('ID and name are required')
          return
        }
        await apiPost('/dashboards', payload)
        setSuccess('Dashboard created')
      }
      resetForm()
      await load()
    } catch (err) {
      setError(err.message)
    }
  }

  const remove = async (dashboard) => {
    if (!(await confirm({ message: `Delete dashboard "${dashboard.name}"?`, variant: 'destructive' }))) return
    setError('')
    try {
      await apiDelete(`/dashboards/${dashboard.id}`)
      setSuccess('Dashboard deleted')
      await load()
    } catch (err) {
      setError(err.message)
    }
  }

  const resetAll = async () => {
    if (!(await confirm({ message: 'Reset all dashboards to defaults? This cannot be undone.', variant: 'destructive' }))) return
    setError('')
    try {
      await apiPost('/me/dashboards/reset', {})
      setSuccess('Dashboards reset')
      await load()
    } catch (err) {
      setError(err.message)
    }
  }

  const formFields = (
    <>
      <Field label="ID">
        <Input
          value={form.id}
          onChange={(e) => setForm({ ...form, id: e.target.value })}
          placeholder="home"
          disabled={!!editingDashboard}
        />
      </Field>
      <Field label="Name">
        <Input
          value={form.name}
          onChange={(e) => setForm({ ...form, name: e.target.value })}
          placeholder="Home Dashboard"
        />
      </Field>
      <Field label="Type">
        <Select
          value={String(form.type)}
          onValueChange={(v) => setForm({ ...form, type: Number(v) })}
        >
          <SelectTrigger className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="1">Normal</SelectItem>
            <SelectItem value="2">Iframe</SelectItem>
            <SelectItem value="4">Search</SelectItem>
            <SelectItem value="6">Tab</SelectItem>
            <SelectItem value="7">Grafana</SelectItem>
          </SelectContent>
        </Select>
      </Field>
      <Field label="Shared">
        <Select
          value={form.shared ? 'true' : 'false'}
          onValueChange={(v) => setForm({ ...form, shared: v === 'true' })}
        >
          <SelectTrigger className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="false">Private</SelectItem>
            <SelectItem value="true">Shared</SelectItem>
          </SelectContent>
        </Select>
      </Field>
      <Field label="Weight">
        <DigitsInput
          value={form.weight}
          onValueChange={(weight) => setForm({ ...form, weight })}
        />
      </Field>
    </>
  )

  return (
    <div>
      <SettingsPageHeader
        title="Dashboards"
        description="Manage your saved dashboards and their layout."
        actions={
          <>
            <Button variant="outline" size="sm" onClick={load} disabled={loading}>
              <RefreshCw className={cn('mr-1.5 size-4', loading && 'animate-spin')} />
              {loading ? 'Loading...' : 'Refresh'}
            </Button>
            {!readOnly && (
              <>
                <Button
                  size="sm"
                  onClick={() => {
                    resetForm()
                    setCreateOpen(true)
                  }}
                >
                  <Plus className="mr-1.5 size-4" />
                  Create
                </Button>
                <Button variant="outline" size="sm" onClick={resetAll}>
                  <RotateCcw className="mr-1.5 size-4" />
                  Reset All
                </Button>
              </>
            )}
          </>
        }
      />

      {error && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      {success && (
        <Alert className="mb-4">
          <AlertDescription>{success}</AlertDescription>
        </Alert>
      )}

      <Card className="mb-6">
        <CardContent className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
          <Input
            placeholder="Search (name or ID)"
            value={filters.search}
            onChange={(e) => setFilters({ ...filters, search: e.target.value })}
          />
          <Select
            value={filters.type || 'all'}
            onValueChange={(v) =>
              setFilters({ ...filters, type: v === 'all' ? '' : v })
            }
          >
            <SelectTrigger className="w-full">
              <SelectValue placeholder="Type" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">Any type</SelectItem>
              <SelectItem value="1">Normal</SelectItem>
              <SelectItem value="2">Iframe</SelectItem>
              <SelectItem value="4">Search</SelectItem>
              <SelectItem value="6">Tab</SelectItem>
              <SelectItem value="7">Grafana</SelectItem>
            </SelectContent>
          </Select>
          <Select
            value={filters.shared || 'all'}
            onValueChange={(v) =>
              setFilters({ ...filters, shared: v === 'all' ? '' : v })
            }
          >
            <SelectTrigger className="w-full">
              <SelectValue placeholder="Shared" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">Any visibility</SelectItem>
              <SelectItem value="true">Shared</SelectItem>
              <SelectItem value="false">Private</SelectItem>
            </SelectContent>
          </Select>
        </CardContent>
      </Card>

      <CrudEditDialog
        open={!!editingDashboard}
        title={
          editingDashboard
            ? settingsEditTitle(
                dashboardTypeLabel(editingDashboard.type),
                editingDashboard.name || editingDashboard.id,
                'Edit Dashboard',
              )
            : 'Edit Dashboard'
        }
        onCancel={resetForm}
        onSave={save}
      >
        {formFields}
      </CrudEditDialog>

      <Card>
        <CardContent className="p-0">
          {dashboards.length === 0 ? (
            <p className="p-6 text-sm text-muted-foreground">No dashboards found.</p>
          ) : filteredDashboards.length === 0 ? (
            <p className="p-6 text-sm text-muted-foreground">
              No dashboards match the current filters.
            </p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>ID</TableHead>
                  <TableHead>Type</TableHead>
                  <TableHead>Shared</TableHead>
                  <TableHead>Weight</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filteredDashboards.map((d) => (
                  <TableRow key={d.id}>
                    <TableCell className="font-medium">{d.name || '—'}</TableCell>
                    <TableCell className="text-muted-foreground">
                      {d.id || d.param || '—'}
                    </TableCell>
                    <TableCell>{dashboardTypeLabel(d.type)}</TableCell>
                    <TableCell>
                      <BoolIndicator value={d.shared} trueLabel="Shared" falseLabel="Private" />
                    </TableCell>
                    <TableCell>{d.weight}</TableCell>
                    <TableCell className="text-right">
                      {!readOnly && (
                        <div className="flex justify-end gap-2">
                          <EditIconButton onClick={() => startEdit(d)} />
                          <DeleteIconButton onClick={() => remove(d)} />
                        </div>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      {!readOnly && (
        <CrudModal title="Create Dashboard" open={createOpen} onClose={resetForm}>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">{formFields}</div>
          <div className="flex justify-end">
            <Button onClick={save}>Create</Button>
          </div>
        </CrudModal>
      )}
    </div>
  )
}
