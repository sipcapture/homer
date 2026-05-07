import { useEffect, useRef, useState } from 'react'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { Copy } from 'lucide-react'
import { EditIconButton, DeleteIconButton } from '@/components/ui/table-action-buttons'
import { apiGet, apiPost, apiPut, apiDelete } from '../api'
import { Button } from '@/components/ui/button'
import { DigitsInput, parseUint } from '@/components/ui/digits-input'
import { Input } from '@/components/ui/input'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/lib/utils'
import CrudTable, {
  type CrudColumn,
  CrudEditDialog,
  CrudModal,
  Field,
  settingsEditTitle,
} from './CrudTable'

function isAuthTokenExpired(row: { expire_date?: unknown }): boolean {
  const raw = row.expire_date
  if (raw == null) return false
  const s = String(raw).trim()
  if (!s) return false
  const t = Date.parse(s)
  if (Number.isNaN(t)) return false
  return t < Date.now()
}

const columns: CrudColumn[] = [
  { key: 'name', label: 'Name' },
  { key: 'active', label: 'Active', type: 'bool' as const, width: '80px' },
  { key: 'create_date', label: 'Created' },
  {
    key: 'expire_date',
    label: 'Expires',
    render: (value, row) => {
      const text = value == null || value === '' ? '—' : String(value)
      const expired = isAuthTokenExpired(row)
      return (
        <span className={cn(expired && 'text-destructive line-through')}>{text}</span>
      )
    },
  },
  { key: 'usage_calls', label: 'Usage', width: '80px' },
  { key: 'limit_calls', label: 'Limit', width: '80px' },
]

const emptyForm = { name: '', expire_date: '', limit_calls: '', active: true }

export default function AuthTokensPanel({ readOnly = false }: { readOnly?: boolean }) {
  const confirm = useConfirm()
  const [items, setItems] = useState<any[]>([])
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')
  const [filters, setFilters] = useState({ name: '', active: '', limit: '100' })
  const [editingItem, setEditingItem] = useState<any>(null)
  const [form, setForm] = useState(emptyForm)
  const [createOpen, setCreateOpen] = useState(false)
  const [createdToken, setCreatedToken] = useState('')
  const filtersLoadImmediateRef = useRef(true)

  const load = async () => {
    setLoading(true)
    setError('')
    try {
      const lim = parseUint(filters.limit, 100)
      const params: Record<string, string | number> = {
        'page[limit]': lim > 0 ? lim : 100,
      }
      if (filters.name) params['filter[name]'] = filters.name
      if (filters.active) params['filter[active]'] = filters.active
      const data = await apiGet('/auth-tokens', params)
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
    setCreatedToken('')
    setSuccess('')
  }

  const startEdit = (item) => {
    setEditingItem(item)
    setForm({
      name: item.name || '',
      expire_date: item.expire_date || '',
      limit_calls: item.limit_calls != null ? String(item.limit_calls) : '',
      active: item.active !== false,
    })
  }

  const save = async () => {
    setSaving(true)
    setError('')
    setSuccess('')
    try {
      if (editingItem) {
        await apiPut(`/auth-tokens/${editingItem.guid}`, {
          name: form.name,
          expire_date: form.expire_date || undefined,
          limit_calls: parseUint(form.limit_calls),
          active: form.active,
        })
        resetForm()
      } else {
        const data: any = await apiPost('/auth-tokens', {
          name: form.name,
          expire_date: form.expire_date || undefined,
          limit_calls: parseUint(form.limit_calls),
          active: form.active,
        })
        const token = data?.data?.token || ''
        if (token) {
          setCreatedToken(token)
          setSuccess('Token created. Copy it now — it will not be shown again.')
        }
      }
      await load()
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const remove = async (item) => {
    if (!(await confirm({ message: `Delete auth token "${item.name}"?`, variant: 'destructive' }))) return
    setError('')
    try {
      await apiDelete(`/auth-tokens/${item.guid}`)
      await load()
    } catch (err) {
      setError(err.message)
    }
  }

  const copyToken = () => {
    navigator.clipboard.writeText(createdToken).catch(() => {})
  }

  const formFields = (
    <>
      <Field label="Name">
        <Input
          value={form.name}
          onChange={(e) => setForm({ ...form, name: e.target.value })}
          placeholder="Token name"
        />
      </Field>
      <Field label="Expire Date">
        <Input
          value={form.expire_date}
          onChange={(e) => setForm({ ...form, expire_date: e.target.value })}
          placeholder="2026-12-31"
        />
      </Field>
      <Field label="Call Limit">
        <DigitsInput
          value={form.limit_calls}
          onValueChange={(limit_calls) => setForm({ ...form, limit_calls })}
        />
      </Field>
      <Field label="Active">
        <Select
          value={form.active ? 'true' : 'false'}
          onValueChange={(v) => setForm({ ...form, active: v === 'true' })}
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
      title="Auth Tokens"
      description="API tokens for authenticating external clients against the Homer API."
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
              ? settingsEditTitle('API token', editingItem.name, 'Edit Token')
              : 'Edit Token'
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
          title="Create Auth Token"
          open={createOpen}
          onClose={resetForm}
        >
          {createdToken ? (
            <div className="space-y-3">
              {success && (
                <Alert>
                  <AlertDescription>{success}</AlertDescription>
                </Alert>
              )}
              <p className="text-sm font-medium text-foreground">Your new API token</p>
              <div className="flex items-center gap-2">
                <Input
                  readOnly
                  value={createdToken}
                  className="flex-1 font-mono text-xs"
                  onClick={(e) => (e.target as HTMLInputElement).select()}
                />
                <Button variant="outline" size="sm" onClick={copyToken}>
                  <Copy className="mr-1.5 size-3.5" />
                  Copy
                </Button>
              </div>
              <p className="text-xs text-muted-foreground">
                Save this token. It will not be displayed again.
              </p>
              <div className="flex justify-end">
                <Button onClick={resetForm}>Done</Button>
              </div>
            </div>
          ) : (
            <>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">{formFields}</div>
              <div className="flex justify-end">
                <Button onClick={save} disabled={saving}>
                  {saving ? 'Creating...' : 'Create'}
                </Button>
              </div>
            </>
          )}
        </CrudModal>
      }
      filters={
        <>
          <Input
            placeholder="Name"
            value={filters.name}
            onChange={(e) => setFilters({ ...filters, name: e.target.value })}
          />
          <Select
            value={filters.active || 'all'}
            onValueChange={(v) => setFilters({ ...filters, active: v === 'all' ? '' : v })}
          >
            <SelectTrigger className="w-full">
              <SelectValue placeholder="Active?" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All</SelectItem>
              <SelectItem value="true">Active</SelectItem>
              <SelectItem value="false">Inactive</SelectItem>
            </SelectContent>
          </Select>
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
