import { useEffect, useRef, useState } from 'react'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { EditIconButton, DeleteIconButton } from '@/components/ui/table-action-buttons'
import { apiGet, apiPost, apiPut, apiDelete } from '../api'
import { Button } from '@/components/ui/button'
import { DigitsInput, parseUint } from '@/components/ui/digits-input'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import CrudTable, {
  CrudEditDialog,
  CrudModal,
  Field,
  settingsEditTitle,
  type CrudColumn,
} from './CrudTable'

const columns: CrudColumn[] = [
  {
    key: 'custom_image',
    label: '',
    width: '52px',
    render: (v) => {
      const u = typeof v === 'string' ? v.trim() : ''
      if (!u) return <span className="text-muted-foreground">—</span>
      return (
        <img
          src={u}
          alt=""
          className="h-8 w-8 rounded border border-border object-cover"
          loading="lazy"
        />
      )
    },
  },
  { key: 'alias', label: 'Alias' },
  { key: 'ip', label: 'IP' },
  { key: 'port', label: 'Port', width: '80px' },
  { key: 'mask', label: 'Mask', width: '80px' },
  { key: 'capture_id', label: 'Capture ID' },
  {
    key: 'tag1',
    label: 'Tags',
    width: '200px',
    render: (_v, row) => {
      const parts = [row.tag1, row.tag2, row.tag3, row.tag4]
        .map((x) => (x != null && String(x).trim() !== '' ? String(x) : null))
        .filter(Boolean) as string[]
      if (!parts.length) return <span className="text-muted-foreground">—</span>
      const s = parts.join(' · ')
      return (
        <span className="block truncate text-sm" title={s}>
          {s}
        </span>
      )
    },
  },
  { key: 'status', label: 'Status', type: 'bool', width: '80px' },
]

const emptyForm = {
  alias: '',
  ip: '',
  port: '',
  mask: '',
  capture_id: '',
  custom_image: '',
  tag1: '',
  tag2: '',
  tag3: '',
  tag4: '',
  status: true,
}

export default function AliasesPanel({ readOnly = false }: { readOnly?: boolean }) {
  const confirm = useConfirm()
  const [items, setItems] = useState<any[]>([])
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [filters, setFilters] = useState({
    alias: '',
    ip: '',
    capture_id: '',
    limit: '100',
  })
  const [editingItem, setEditingItem] = useState<any>(null)
  const [form, setForm] = useState(emptyForm)
  const [createOpen, setCreateOpen] = useState(false)
  /** First run after mount: load immediately; later filter changes are debounced. */
  const filtersLoadImmediateRef = useRef(true)

  const load = async () => {
    setLoading(true)
    setError('')
    try {
      const lim = parseUint(filters.limit, 100)
      const params: Record<string, string | number> = { 'page[limit]': lim > 0 ? lim : 100 }
      if (filters.alias) params['filter[alias]'] = filters.alias
      if (filters.ip) params['filter[ip]'] = filters.ip
      if (filters.capture_id) params['filter[capture_id]'] = filters.capture_id
      const data = await apiGet('/aliases', params)
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
      alias: item.alias || '',
      ip: item.ip || '',
      port: item.port != null ? String(item.port) : '',
      mask: item.mask != null ? String(item.mask) : '',
      capture_id: item.capture_id || '',
      custom_image: item.custom_image || '',
      tag1: item.tag1 || '',
      tag2: item.tag2 || '',
      tag3: item.tag3 || '',
      tag4: item.tag4 || '',
      status: item.status !== false,
    })
  }

  const save = async () => {
    setSaving(true)
    setError('')
    try {
      const body = {
        ...form,
        port: parseUint(form.port),
        mask: parseUint(form.mask),
      }
      if (editingItem) {
        await apiPut(`/aliases/${editingItem.guid}`, body)
      } else {
        await apiPost('/aliases', body)
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
    if (!(await confirm({ message: `Delete alias "${item.alias}"?`, variant: 'destructive' }))) return
    setError('')
    try {
      await apiDelete(`/aliases/${item.guid}`)
      await load()
    } catch (err) {
      setError(err.message)
    }
  }

  const formFields = (
    <>
      <Field label="Alias">
        <Input
          value={form.alias}
          onChange={(e) => setForm({ ...form, alias: e.target.value })}
          placeholder="Alias name"
        />
      </Field>
      <Field label="IP">
        <Input
          value={form.ip}
          onChange={(e) => setForm({ ...form, ip: e.target.value })}
          placeholder="IP address"
        />
      </Field>
      <Field label="Port">
        <DigitsInput
          value={form.port}
          onValueChange={(port) => setForm({ ...form, port })}
        />
      </Field>
      <Field label="Mask">
        <DigitsInput
          value={form.mask}
          onValueChange={(mask) => setForm({ ...form, mask })}
        />
      </Field>
      <Field label="Capture ID">
        <Input
          value={form.capture_id}
          onChange={(e) => setForm({ ...form, capture_id: e.target.value })}
        />
      </Field>
      <Field label="Custom image URL">
        <Input
          value={form.custom_image}
          onChange={(e) => setForm({ ...form, custom_image: e.target.value })}
          placeholder="https://…"
        />
      </Field>
      <Field label="Tag 1 (e.g. type)">
        <Input
          value={form.tag1}
          onChange={(e) => setForm({ ...form, tag1: e.target.value })}
        />
      </Field>
      <Field label="Tag 2 (e.g. OS)">
        <Input
          value={form.tag2}
          onChange={(e) => setForm({ ...form, tag2: e.target.value })}
        />
      </Field>
      <Field label="Tag 3">
        <Input
          value={form.tag3}
          onChange={(e) => setForm({ ...form, tag3: e.target.value })}
        />
      </Field>
      <Field label="Tag 4">
        <Input
          value={form.tag4}
          onChange={(e) => setForm({ ...form, tag4: e.target.value })}
        />
      </Field>
      <Field label="Status">
        <Select
          value={form.status ? 'true' : 'false'}
          onValueChange={(v) => setForm({ ...form, status: v === 'true' })}
        >
          <SelectTrigger className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="true">Active</SelectItem>
            <SelectItem value="false">Inactive</SelectItem>
          </SelectContent>
        </Select>
      </Field>
    </>
  )

  return (
    <CrudTable
      title="IP Aliases"
      description="Map friendly aliases and CIDR ranges to IP addresses. Optional image URL and tags (type, OS, …) are returned on search rows as aliasSrc_* / aliasDst_* when enrichment matches."
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
              ? settingsEditTitle('IP alias', editingItem.alias, 'Edit Alias')
              : 'Edit Alias'
          }
          onCancel={resetForm}
          onSave={save}
          saving={saving}
        >
          {formFields}
        </CrudEditDialog>
      }
      createForm={
        <CrudModal title="Create Alias" open={createOpen} onClose={resetForm}>
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
            placeholder="Alias"
            value={filters.alias}
            onChange={(e) => setFilters({ ...filters, alias: e.target.value })}
          />
          <Input
            placeholder="IP"
            value={filters.ip}
            onChange={(e) => setFilters({ ...filters, ip: e.target.value })}
          />
          <Input
            placeholder="Capture ID"
            value={filters.capture_id}
            onChange={(e) => setFilters({ ...filters, capture_id: e.target.value })}
          />
          <DigitsInput
            placeholder="Limit"
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
