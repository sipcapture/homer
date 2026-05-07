import { useEffect, useRef, useState } from 'react'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { EditIconButton, DeleteIconButton } from '@/components/ui/table-action-buttons'
import { apiGet, apiPost, apiPut, apiDelete } from '../api'
import { Button } from '@/components/ui/button'
import { DigitsInput, parseUint } from '@/components/ui/digits-input'
import { Input } from '@/components/ui/input'
import CrudTable, {
  CrudEditDialog,
  CrudModal,
  Field,
  JsonEditorField,
  settingsEditTitle,
} from './CrudTable'

const columns = [
  { key: 'category', label: 'Category' },
  { key: 'param', label: 'Param' },
  { key: 'data', label: 'Data', type: 'json' as const },
]

const emptyForm = { category: '', param: '', data: '{}' }

export default function AdvancedPanel({ readOnly = false }: { readOnly?: boolean }) {
  const confirm = useConfirm()
  const [items, setItems] = useState<any[]>([])
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [editingItem, setEditingItem] = useState<any>(null)
  const [createOpen, setCreateOpen] = useState(false)
  const [form, setForm] = useState(emptyForm)
  const [filters, setFilters] = useState({ category: '', param: '', limit: '100' })
  const filtersLoadImmediateRef = useRef(true)

  const load = async () => {
    setLoading(true)
    setError('')
    try {
      const lim = parseUint(filters.limit, 100)
      const params: Record<string, string | number> = {
        'page[limit]': lim > 0 ? lim : 100,
      }
      if (filters.category) params['filter[category]'] = filters.category
      if (filters.param) params['filter[param]'] = filters.param
      const data = await apiGet('/advanced', params)
      setItems(data?.data?.items || [])
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    if (filtersLoadImmediateRef.current) {
      filtersLoadImmediateRef.current = false
      void load()
      return
    }
    const id = window.setTimeout(() => {
      void load()
    }, 400)
    return () => window.clearTimeout(id)
    // eslint-disable-next-line react-hooks/exhaustive-deps -- load reads latest filters
  }, [filters])

  const resetForm = () => {
    setEditingItem(null)
    setCreateOpen(false)
    setForm(emptyForm)
  }

  const startEdit = (item) => {
    setEditingItem(item)
    setForm({
      category: item.category || '',
      param: item.param || '',
      data:
        typeof item.data === 'object'
          ? JSON.stringify(item.data, null, 2)
          : item.data || '{}',
    })
  }

  const save = async () => {
    setSaving(true)
    setError('')
    try {
      const payload = {
        category: form.category,
        param: form.param,
        data: JSON.parse(form.data || '{}'),
      }
      if (editingItem) {
        await apiPut(`/advanced/${editingItem.guid}`, payload)
      } else {
        await apiPost('/advanced', payload)
      }
      resetForm()
      await load()
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const remove = async (item) => {
    if (!(await confirm({ message: `Delete advanced setting "${item.category}:${item.param}"?`, variant: 'destructive' }))) return
    setError('')
    try {
      await apiDelete(`/advanced/${item.guid}`)
      await load()
    } catch (err) {
      setError(err.message)
    }
  }

  const formFields = (
    <>
      <Field label="Category">
        <Input
          value={form.category}
          onChange={(e) => setForm({ ...form, category: e.target.value })}
        />
      </Field>
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
    </>
  )

  return (
    <CrudTable
      title="Advanced Settings"
      description="Key-value configuration overrides stored as JSON blobs."
      columns={columns}
      items={items}
      loading={loading}
      onLoad={load}
      error={error}
      onCreateOpen={
        readOnly
          ? undefined
          : () => {
              resetForm()
              setCreateOpen(true)
            }
      }
      showActions={!readOnly}
      editForm={
        <CrudEditDialog
          open={!!editingItem}
          title={
            editingItem
              ? settingsEditTitle(editingItem.category, editingItem.param)
              : 'Edit Advanced Setting'
          }
          onCancel={resetForm}
          onSave={save}
          saving={saving}
        >
          {formFields}
        </CrudEditDialog>
      }
      createForm={
        <CrudModal
          title="Create Advanced Setting"
          open={createOpen}
          onClose={resetForm}
        >
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">{formFields}</div>
          <div className="flex justify-end">
            <Button onClick={save} disabled={saving}>
              {saving ? 'Creating...' : 'Create'}
            </Button>
          </div>
        </CrudModal>
      }
      filters={
        <>
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
          <DigitsInput
            min={1}
            max={1000}
            value={filters.limit}
            onValueChange={(limit) => setFilters({ ...filters, limit })}
          />
        </>
      }
    >
      {!readOnly
        ? (item) => (
            <>
              <EditIconButton onClick={() => startEdit(item)} />
              <DeleteIconButton onClick={() => remove(item)} />
            </>
          )
        : undefined}
    </CrudTable>
  )
}
