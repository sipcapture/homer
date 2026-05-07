import { useEffect, useRef, useState } from 'react'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { EditIconButton, DeleteIconButton } from '@/components/ui/table-action-buttons'
import { apiGet, apiPost, apiPut, apiDelete } from '../api'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { DigitsInput, parseUint } from '@/components/ui/digits-input'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import ScriptEditor from '@/components/ui/script-editor'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import CrudTable, { Field, settingsEditTitle } from './CrudTable'

const columns = [
  { key: 'profile', label: 'Profile' },
  { key: 'hep_alias', label: 'HEP Alias' },
  { key: 'type', label: 'Type' },
  { key: 'hepid', label: 'HEP ID', width: '80px' },
  { key: 'status', label: 'Status', type: 'bool' as const, width: '80px' },
]

const emptyForm = {
  profile: '',
  hep_alias: '',
  type: '',
  hepid: '',
  status: true,
  script: '',
}

export default function ScriptsPanel({ readOnly = false }: { readOnly?: boolean }) {
  const confirm = useConfirm()
  const [items, setItems] = useState<any[]>([])
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [editingItem, setEditingItem] = useState<any>(null)
  const [createOpen, setCreateOpen] = useState(false)
  const [form, setForm] = useState(emptyForm)
  const [filters, setFilters] = useState({ profile: '', type: '', limit: '100' })
  const scriptDialogOpen = !readOnly && (createOpen || !!editingItem)
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
      if (filters.type) params['filter[type]'] = filters.type
      const data = await apiGet('/scripts', params)
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
    setCreateOpen(false)
    setEditingItem(item)
    setForm({
      profile: item.profile || '',
      hep_alias: item.hep_alias || '',
      type: item.type || '',
      hepid: item.hepid != null ? String(item.hepid) : '',
      status: item.status !== false,
      script: item.script || '',
    })
  }

  const save = async () => {
    setSaving(true)
    setError('')
    try {
      const payload = {
        profile: form.profile,
        hep_alias: form.hep_alias,
        type: form.type,
        hepid: parseUint(form.hepid),
        status: !!form.status,
        script: form.script,
      }
      if (editingItem) {
        await apiPut(`/scripts/${editingItem.guid}`, payload)
      } else {
        await apiPost('/scripts', payload)
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
    if (!(await confirm({ message: `Delete script "${item.profile}"?`, variant: 'destructive' }))) return
    setError('')
    try {
      await apiDelete(`/scripts/${item.guid}`)
      await load()
    } catch (err) {
      setError(err.message)
    }
  }

  const metaFields = (
    <>
      <Field label="Profile">
        <Input
          value={form.profile}
          onChange={(e) => setForm({ ...form, profile: e.target.value })}
        />
      </Field>
      <Field label="HEP Alias">
        <Input
          value={form.hep_alias}
          onChange={(e) => setForm({ ...form, hep_alias: e.target.value })}
        />
      </Field>
      <Field label="Type">
        <Input
          value={form.type}
          onChange={(e) => setForm({ ...form, type: e.target.value })}
        />
      </Field>
      <Field label="HEP ID">
        <DigitsInput
          value={form.hepid}
          onValueChange={(hepid) => setForm({ ...form, hepid })}
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
            <SelectItem value="true">Enabled</SelectItem>
            <SelectItem value="false">Disabled</SelectItem>
          </SelectContent>
        </Select>
      </Field>
    </>
  )

  const scriptBlock = (
    <Field label="Lua script" full>
      <div className="flex flex-col gap-2">
        <span className="text-muted-foreground text-xs">
          Lua (e.g. type <span className="font-mono">correlation</span> for call-id correlation)
        </span>
        <div className="min-h-[280px] w-full min-w-0 shrink-0 overflow-hidden">
          <ScriptEditor
            value={form.script}
            onChange={(v) => setForm({ ...form, script: v })}
            readOnly={readOnly}
            className="w-full min-h-0"
            height="min(55vh, 560px)"
            minHeight={280}
            placeholder={'function correlate(data, nodes)\n  return {}\nend'}
          />
        </div>
      </div>
    </Field>
  )

  const scriptDialogContentClass = cn(
    'flex max-h-[92vh] min-h-0 flex-col gap-0 overflow-hidden border p-0 pt-10 pb-0 sm:max-w-none',
    'min-h-[min(480px,90vh)] min-w-[min(94vw,480px)] max-w-[min(98vw,1680px)] w-[min(94vw,960px)]',
    'resize',
  )

  return (
    <>
      <Dialog
        open={scriptDialogOpen}
        onOpenChange={(o) => {
          if (!o) resetForm()
        }}
      >
        <DialogContent className={scriptDialogContentClass} showCloseButton>
          <DialogHeader className="border-b border-border px-4 pb-3">
            <DialogTitle>
              {editingItem
                ? settingsEditTitle(
                    editingItem.type,
                    [editingItem.hep_alias, editingItem.profile].filter(Boolean).join(' · ') ||
                      editingItem.profile,
                    'Edit Script',
                  )
                : 'Create Script'}
            </DialogTitle>
          </DialogHeader>
          <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-hidden px-4 py-4">
            <div className="grid shrink-0 grid-cols-1 gap-4 sm:grid-cols-2">{metaFields}</div>
            <div className="flex min-h-[280px] min-w-0 flex-1 flex-col overflow-hidden">{scriptBlock}</div>
          </div>
          <DialogFooter className="border-t border-border px-4 py-3 sm:justify-end">
            <Button variant="outline" type="button" onClick={() => resetForm()}>
              Cancel
            </Button>
            <Button type="button" onClick={save} disabled={saving}>
              {saving ? 'Saving...' : editingItem ? 'Save' : 'Create'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <CrudTable
        title="Scripts"
        description="Lua scripts applied to incoming packets before storage."
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
        filters={
          <>
            <Input
              placeholder="Profile"
              value={filters.profile}
              onChange={(e) => setFilters({ ...filters, profile: e.target.value })}
            />
            <Input
              placeholder="Type"
              value={filters.type}
              onChange={(e) => setFilters({ ...filters, type: e.target.value })}
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
    </>
  )
}
