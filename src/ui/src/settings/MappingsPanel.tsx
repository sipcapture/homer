import { useEffect, useRef, useState } from 'react'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { Code2 } from 'lucide-react'
import { EditIconButton, DeleteIconButton } from '@/components/ui/table-action-buttons'
import { apiGet, apiPost, apiPut, apiDelete } from '../api'
import { Button } from '@/components/ui/button'
import { DigitsInput, parseUint } from '@/components/ui/digits-input'
import { Input } from '@/components/ui/input'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { ScrollArea } from '@/components/ui/scroll-area'
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
  { key: 'partid', label: 'Part ID', width: '80px' },
  { key: 'version', label: 'Version', width: '80px' },
  { key: 'retention', label: 'Retention', width: '90px' },
]

const emptyForm = {
  profile: '',
  hepid: '',
  hep_alias: '',
  partid: '10',
  version: '1',
  retention: '14',
  partition_step: '3600',
  create_index: '{}',
  create_table: '',
  fields_mapping: '{}',
  fields_settings: '{}',
  schema_mapping: '{}',
  schema_settings: '{}',
}

export default function MappingsPanel({ readOnly = false }: { readOnly?: boolean }) {
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
  const [viewItem, setViewItem] = useState<any>(null)
  /** First run after mount: load immediately; later filter changes are debounced. */
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
      const data = await apiGet('/mappings', params)
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
      partid: item.partid != null ? String(item.partid) : '10',
      version: item.version != null ? String(item.version) : '1',
      retention: item.retention != null ? String(item.retention) : '14',
      partition_step:
        item.partition_step != null ? String(item.partition_step) : '3600',
      create_index: toFormJson(item.create_index),
      create_table: item.create_table || '',
      fields_mapping: toFormJson(item.fields_mapping),
      fields_settings: toFormJson(item.fields_settings),
      schema_mapping: toFormJson(item.schema_mapping),
      schema_settings: toFormJson(item.schema_settings),
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
        partid: parseUint(form.partid, 10),
        version: parseUint(form.version, 1),
        retention: parseUint(form.retention, 14),
        partition_step: parseUint(form.partition_step, 3600),
        create_index: parseJson(form.create_index),
        create_table: form.create_table,
        fields_mapping: parseJson(form.fields_mapping),
        fields_settings: parseJson(form.fields_settings),
        schema_mapping: parseJson(form.schema_mapping),
        schema_settings: parseJson(form.schema_settings),
      }
      if (editingItem) {
        await apiPut(`/mappings/${editingItem.guid}`, payload)
      } else {
        await apiPost('/mappings', payload)
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
    if (!(await confirm({ message: `Delete mapping "${item.profile}" (hepid ${item.hepid})?`, variant: 'destructive' }))) return
    setError('')
    try {
      await apiDelete(`/mappings/${item.guid}`)
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
        <DigitsInput
          value={form.hepid}
          onValueChange={(hepid) => setForm({ ...form, hepid })}
        />
      </Field>
      <Field label="HEP Alias">
        <Input
          value={form.hep_alias}
          onChange={(e) => setForm({ ...form, hep_alias: e.target.value })}
          placeholder="SIP"
        />
      </Field>
      <Field label="Part ID">
        <DigitsInput
          value={form.partid}
          onValueChange={(partid) => setForm({ ...form, partid })}
        />
      </Field>
      <Field label="Version">
        <DigitsInput
          value={form.version}
          onValueChange={(version) => setForm({ ...form, version })}
        />
      </Field>
      <Field label="Retention (days)">
        <DigitsInput
          value={form.retention}
          onValueChange={(retention) => setForm({ ...form, retention })}
        />
      </Field>
      <Field label="Partition Step (seconds)">
        <DigitsInput
          value={form.partition_step}
          onValueChange={(partition_step) => setForm({ ...form, partition_step })}
        />
      </Field>
      <JsonEditorField
        label="Fields Mapping"
        value={form.fields_mapping}
        onChange={(v) =>
          setForm({
            ...form,
            fields_mapping: typeof v === 'string' ? v : JSON.stringify(v, null, 2),
          })
        }
      />
      <JsonEditorField
        label="Fields Settings"
        value={form.fields_settings}
        onChange={(v) =>
          setForm({
            ...form,
            fields_settings: typeof v === 'string' ? v : JSON.stringify(v, null, 2),
          })
        }
      />
      <JsonEditorField
        label="Schema Mapping"
        value={form.schema_mapping}
        onChange={(v) =>
          setForm({
            ...form,
            schema_mapping: typeof v === 'string' ? v : JSON.stringify(v, null, 2),
          })
        }
      />
      <JsonEditorField
        label="Schema Settings"
        value={form.schema_settings}
        onChange={(v) =>
          setForm({
            ...form,
            schema_settings: typeof v === 'string' ? v : JSON.stringify(v, null, 2),
          })
        }
      />
    </>
  )

  return (
    <>
      <CrudTable
        title="Protocol Mappings"
        description="Per-profile field and schema mappings applied to captured traffic."
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
                    'Edit Mapping',
                  )
                : 'Edit Mapping'
            }
            onCancel={resetForm}
            onSave={save}
            saving={saving}
            className="w-[min(94vw,960px)] max-w-[min(98vw,1080px)] sm:max-w-none"
          >
            {formFields}
          </CrudEditDialog>
        }
        createForm={
          <CrudModal
            title="Create Mapping"
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
              onChange={(e) =>
                setFilters({ ...filters, hep_alias: e.target.value })
              }
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
        {(item) => (
          <>
            <Button
              variant="outline"
              size="icon-xs"
              aria-label="View JSON"
              title="View JSON"
              type="button"
              onClick={() => setViewItem(item)}
            >
              <Code2 className="size-3" aria-hidden />
            </Button>
            {!readOnly && (
              <>
                <EditIconButton onClick={() => startEdit(item)} />
                <DeleteIconButton onClick={() => remove(item)} />
              </>
            )}
          </>
        )}
      </CrudTable>

      <Dialog open={!!viewItem} onOpenChange={(o) => !o && setViewItem(null)}>
        <DialogContent className="max-w-3xl">
          <DialogHeader>
            <DialogTitle>
              {viewItem?.profile} (hepid {viewItem?.hepid})
            </DialogTitle>
          </DialogHeader>
          <ScrollArea className="max-h-[60vh]">
            <pre className="whitespace-pre-wrap bg-muted/40 p-3 font-mono text-[11px]">
              {viewItem ? JSON.stringify(viewItem, null, 2) : ''}
            </pre>
          </ScrollArea>
        </DialogContent>
      </Dialog>
    </>
  )
}
