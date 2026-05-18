import { useCallback, useEffect, useMemo, useState } from 'react'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { Plus, RefreshCw } from 'lucide-react'
import { EditIconButton, DeleteIconButton } from '@/components/ui/table-action-buttons'
import { apiGet, apiPut, apiDelete } from '../api'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { cn } from '@/lib/utils'
import { CrudEditDialog, Field, JsonEditorField, settingsEditTitle } from './CrudTable'
import { SettingsPageHeader } from './SettingsPageHeader'
import { SortableTableHead } from './SortableTableHead'
import { useTableSort } from './useTableSort'

export default function UserSettingsPanel() {
  const confirm = useConfirm()
  const [items, setItems] = useState<any[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')
  const [editingItem, setEditingItem] = useState<any>(null)
  const [form, setForm] = useState({ param: '', data: '{}' })
  const [createOpen, setCreateOpen] = useState(false)
  const [saveCategory, setSaveCategory] = useState('')
  const [saveParam, setSaveParam] = useState('')
  const [saveData, setSaveData] = useState('{}')
  const [filters, setFilters] = useState({ search: '', category: '', param: '' })

  const load = async () => {
    setLoading(true)
    setError('')
    try {
      const data = await apiGet('/me/settings')
      setItems(data?.data?.items || [])
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [])

  const filteredItems = useMemo(() => {
    let list = items
    const cat = filters.category.trim().toLowerCase()
    if (cat) {
      list = list.filter((it) => String(it.category ?? '').toLowerCase().includes(cat))
    }
    const par = filters.param.trim().toLowerCase()
    if (par) {
      list = list.filter((it) => String(it.param ?? '').toLowerCase().includes(par))
    }
    const q = filters.search.trim().toLowerCase()
    if (q) {
      list = list.filter((it) => {
        const c = String(it.category ?? '').toLowerCase()
        const p = String(it.param ?? '').toLowerCase()
        const d =
          it.data == null
            ? ''
            : typeof it.data === 'object'
              ? JSON.stringify(it.data)
              : String(it.data)
        const dl = d.toLowerCase()
        return c.includes(q) || p.includes(q) || dl.includes(q)
      })
    }
    return list
  }, [items, filters])

  const getSortValue = useCallback((item: any, columnKey: string) => {
    if (columnKey === 'data') {
      const v = item.data
      if (v == null) return ''
      return typeof v === 'object' ? JSON.stringify(v) : String(v)
    }
    return item[columnKey]
  }, [])

  const { sortCol, sortDir, toggleSort, sortedItems: tableRows } = useTableSort(
    filteredItems,
    getSortValue,
  )

  const startEdit = (item) => {
    setEditingItem(item)
    setForm({
      param: item.param || '',
      data:
        typeof item.data === 'object'
          ? JSON.stringify(item.data, null, 2)
          : item.data || '{}',
    })
  }

  const save = async () => {
    if (!editingItem) return
    setError('')
    setSuccess('')
    try {
      const dataObj = JSON.parse(form.data)
      await apiPut(`/me/settings/${editingItem.category}`, {
        param: form.param,
        data: dataObj,
      })
      setSuccess('Setting saved')
      setEditingItem(null)
      await load()
    } catch (err) {
      setError(err.message)
    }
  }

  const remove = async (item) => {
    if (!(await confirm({ message: `Delete setting "${item.category}" (${item.param})?`, variant: 'destructive' }))) return
    setError('')
    try {
      await apiDelete(`/me/settings/${item.category}`)
      setSuccess('Setting deleted')
      await load()
    } catch (err) {
      setError(err.message)
    }
  }

  const resetCreate = () => {
    setCreateOpen(false)
    setSaveCategory('')
    setSaveParam('')
    setSaveData('{}')
  }

  const saveNew = async () => {
    if (!saveCategory || !saveParam) {
      setError('Category and param are required')
      return
    }
    setError('')
    setSuccess('')
    try {
      const dataObj = JSON.parse(saveData)
      await apiPut(`/me/settings/${saveCategory}`, {
        param: saveParam,
        data: dataObj,
      })
      setSuccess('Setting saved')
      resetCreate()
      await load()
    } catch (err) {
      setError(err.message)
    }
  }

  const addFormFields = (
    <>
      <Field label="Category">
        <Input
          value={saveCategory}
          onChange={(e) => setSaveCategory(e.target.value)}
          placeholder="dashboard"
        />
      </Field>
      <Field label="Param">
        <Input
          value={saveParam}
          onChange={(e) => setSaveParam(e.target.value)}
          placeholder="home"
        />
      </Field>
      <JsonEditorField
        label="Data"
        value={saveData}
        onChange={(v) =>
          setSaveData(typeof v === 'string' ? v : JSON.stringify(v, null, 2))
        }
      />
    </>
  )

  return (
    <div className="space-y-6">
      <SettingsPageHeader
        title="User Settings"
        description="Per-user preferences stored as JSON by category and param."
        actions={
          <>
            <Button variant="outline" size="sm" onClick={load} disabled={loading}>
              <RefreshCw className={cn('mr-1.5 size-4', loading && 'animate-spin')} />
              {loading ? 'Loading...' : 'Refresh'}
            </Button>
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              <Plus className="mr-1.5 size-4" />
              Add Settings
            </Button>
          </>
        }
      />

      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      {success && (
        <Alert>
          <AlertDescription>{success}</AlertDescription>
        </Alert>
      )}

      <Card className="mb-6">
        <CardContent className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
          <Input
            placeholder="Search (category, param, or data)"
            value={filters.search}
            onChange={(e) => setFilters({ ...filters, search: e.target.value })}
          />
          <Input
            placeholder="Category"
            value={filters.category}
            onChange={(e) => setFilters({ ...filters, category: e.target.value })}
          />
          <Input
            placeholder="Param"
            value={filters.param}
            onChange={(e) => setFilters({ ...filters, param: e.target.value })}
          />
        </CardContent>
      </Card>

      <CrudEditDialog
        open={!!editingItem}
        title={
          editingItem
            ? settingsEditTitle(editingItem.category, editingItem.param)
            : 'Edit setting'
        }
        onCancel={() => setEditingItem(null)}
        onSave={save}
        saveLabel="Save"
      >
        <Field label="Param">
          <Input
            value={form.param}
            onChange={(e) => setForm({ ...form, param: e.target.value })}
          />
        </Field>
        <JsonEditorField
          label="Data"
          value={form.data}
          onChange={(v) =>
            setForm({
              ...form,
              data: typeof v === 'string' ? v : JSON.stringify(v, null, 2),
            })
          }
        />
      </CrudEditDialog>

      <CrudEditDialog
        open={createOpen}
        title="Add Settings"
        onCancel={resetCreate}
        onSave={saveNew}
        saveLabel="Save Setting"
      >
        {addFormFields}
      </CrudEditDialog>

      <Card>
        <CardHeader>
          <CardTitle>Stored Settings</CardTitle>
          <CardDescription>
            Use Add Settings to create a row, or edit / delete from the table.
          </CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          {items.length === 0 ? (
            <p className="p-6 text-sm text-muted-foreground">No user settings found.</p>
          ) : filteredItems.length === 0 ? (
            <p className="p-6 text-sm text-muted-foreground">
              No settings match the current filters.
            </p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <SortableTableHead
                    columnKey="category"
                    label="Category"
                    sortCol={sortCol}
                    sortDir={sortDir}
                    onSort={toggleSort}
                  />
                  <SortableTableHead
                    columnKey="param"
                    label="Param"
                    sortCol={sortCol}
                    sortDir={sortDir}
                    onSort={toggleSort}
                  />
                  <SortableTableHead
                    columnKey="data"
                    label="Data"
                    sortCol={sortCol}
                    sortDir={sortDir}
                    onSort={toggleSort}
                  />
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {tableRows.map((item, idx) => (
                  <TableRow key={item.guid || idx}>
                    <TableCell className="font-medium">{item.category || '—'}</TableCell>
                    <TableCell>{item.param || '—'}</TableCell>
                    <TableCell
                      className="max-w-sm truncate font-mono text-xs text-muted-foreground"
                      title={JSON.stringify(item.data)}
                    >
                      {typeof item.data === 'object'
                        ? JSON.stringify(item.data)
                        : String(item.data || '')}
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-2">
                        <EditIconButton onClick={() => startEdit(item)} />
                        <DeleteIconButton onClick={() => remove(item)} />
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
