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
  { key: 'profile', label: 'Profile' },
  { key: 'hepid', label: 'HEP ID', width: '80px' },
  { key: 'hep_alias', label: 'HEP Alias' },
  { key: 'version', label: 'Version', width: '80px' },
]

const emptyForm = {
  profile: '',
  hepid: '',
  hep_alias: '',
  version: '1',
  mapping: '{}',
}

export default function HepsubsPanel({ readOnly = false }: { readOnly?: boolean }) {
  const confirm = useConfirm()
  const [items, setItems] = useState<any[]>([])
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [filters, setFilters] = useState({
    profile: '',
    hep_alias: '',
    hepid: '',
    limit: '100',
  })
  const [editingItem, setEditingItem] = useState<any>(null)
  const [form, setForm] = useState(emptyForm)
  const [createOpen, setCreateOpen] = useState(false)
  const filtersLoadImmediateRef = useRef(true)

  const load = async () => {
    setLoading(true)
    setError('')
    try {
      const lim = parseUint(filters.limit, 100)
      const params: Record<string, string | number> = {
        'page[limit]': lim > 0 ? lim : 100,
      }
      if (filters.profile) params['filter[profile]'] = filters.profile
      if (filters.hep_alias) params['filter[hep_alias]'] = filters.hep_alias
      if (filters.hepid) params['filter[hepid]'] = filters.hepid
      const data = await apiGet('/hepsubs', params)
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

  const toFormJson = (val: unknown) => {
    if (!val) return '{}'
    if (typeof val === 'string') return val
    return JSON.stringify(val, null, 2)
  }

  const startEdit = (item) => {
    setEditingItem(item)
    setForm({
      profile: item.profile || '',
      hepid: item.hepid != null ? String(item.hepid) : '',
      hep_alias: item.hep_alias || '',
      version: item.version != null ? String(item.version) : '1',
      mapping: toFormJson(item.mapping),
    })
  }

  const parseJson = (str: string) => {
    try {
      return JSON.parse(str)
    } catch {
      return str
    }
  }

  const save = async () => {
    setSaving(true)
    setError('')
    try {
      const payload = {
        profile: form.profile,
        hepid: parseUint(form.hepid),
        hep_alias: form.hep_alias,
        version: parseUint(form.version, 1),
        mapping: parseJson(form.mapping),
      }
      if (editingItem) {
        await apiPut(`/hepsubs/${editingItem.guid}`, payload)
      } else {
        await apiPost('/hepsubs', payload)
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
    if (!(await confirm({ message: `Delete hepsub "${item.profile}" (hepid ${item.hepid})?`, variant: 'destructive' }))) return
    setError('')
    try {
      await apiDelete(`/hepsubs/${item.guid}`)
      await load()
    } catch (err) {
      setError(err.message)
    }
  }

  const formFields = (
    <>
      <Field label="Profile">
        <Input
          value={form.profile}
          onChange={(e) => setForm({ ...form, profile: e.target.value })}
          placeholder="call"
        />
      </Field>
      <Field label="HEP ID">
        <DigitsInput value={form.hepid} onValueChange={(hepid) => setForm({ ...form, hepid })} />
      </Field>
      <Field label="HEP Alias">
        <Input
          value={form.hep_alias}
          onChange={(e) => setForm({ ...form, hep_alias: e.target.value })}
          placeholder="SIP"
        />
      </Field>
      <Field label="Version">
        <DigitsInput
          value={form.version}
          onValueChange={(version) => setForm({ ...form, version })}
        />
      </Field>
      <JsonEditorField
        label="Mapping"
        value={form.mapping}
        onChange={(v) =>
          setForm({
            ...form,
            mapping: typeof v === 'string' ? v : JSON.stringify(v, null, 2),
          })
        }
      />
    </>
  )

  return (
    <CrudTable
      title="HEP Subscriptions"
      description="HEP protocol subscription mappings that route incoming packets to profiles."
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
              ? settingsEditTitle(
                  editingItem.hep_alias,
                  `${editingItem.profile} (HEP ${editingItem.hepid})`,
                  'Edit HEP Subscription',
                )
              : 'Edit HEP Subscription'
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
          title="Create HEP Subscription"
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
            placeholder="Profile"
            value={filters.profile}
            onChange={(e) => setFilters({ ...filters, profile: e.target.value })}
          />
          <Input
            placeholder="HEP Alias"
            value={filters.hep_alias}
            onChange={(e) => setFilters({ ...filters, hep_alias: e.target.value })}
          />
          <DigitsInput
            placeholder="HEP ID"
            value={filters.hepid}
            onValueChange={(hepid) => setFilters({ ...filters, hepid })}
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
