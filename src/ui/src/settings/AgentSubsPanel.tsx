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
} from './CrudTable'

const columns = [
  { key: 'host', label: 'Host' },
  { key: 'port', label: 'Port', width: '80px' },
  { key: 'protocol', label: 'Protocol', width: '90px' },
  { key: 'type', label: 'Type' },
  { key: 'node', label: 'Node' },
  { key: 'active', label: 'Active', width: '70px' },
  { key: 'expire_date', label: 'Expires' },
]

const emptyForm = {
  host: '',
  port: '',
  protocol: 'udp',
  path: '',
  node: '',
  type: '',
  ttl: '300',
  active: 1,
}

export default function AgentSubsPanel({ readOnly = false }: { readOnly?: boolean }) {
  const confirm = useConfirm()
  const [items, setItems] = useState<any[]>([])
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [filters, setFilters] = useState({ type: '', node: '', limit: '100' })
  const [editingItem, setEditingItem] = useState<any>(null)
  const [form, setForm] = useState(emptyForm)
  const [createOpen, setCreateOpen] = useState(false)
  const [agentTypes, setAgentTypes] = useState<string[]>([])
  const filtersLoadImmediateRef = useRef(true)

  const load = async () => {
    setLoading(true)
    setError('')
    try {
      const lim = parseUint(filters.limit, 100)
      const params: Record<string, string | number> = {
        'page[limit]': lim > 0 ? lim : 100,
      }
      if (filters.type) params['filter[type]'] = filters.type
      if (filters.node) params['filter[node]'] = filters.node
      const data = await apiGet('/agents/subscriptions', params)
      setItems(data?.data?.items || [])
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  const loadTypes = async () => {
    try {
      const data = await apiGet('/agents/types')
      setAgentTypes(data?.data?.items || [])
    } catch {
      // ignore
    }
  }

  useEffect(() => {
    void loadTypes()
  }, [])

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
      host: item.host || '',
      port: item.port != null ? String(item.port) : '',
      protocol: item.protocol || 'udp',
      path: item.path || '',
      node: item.node || '',
      type: item.type || '',
      ttl: item.ttl != null ? String(item.ttl) : '300',
      active: item.active ?? 1,
    })
  }

  const save = async () => {
    setSaving(true)
    setError('')
    try {
      const payload = {
        host: form.host,
        port: parseUint(form.port),
        protocol: form.protocol,
        path: form.path,
        node: form.node,
        type: form.type,
        ttl: parseUint(form.ttl, 300),
        active: Number(form.active),
      }
      if (editingItem) {
        await apiPut(`/agents/subscriptions/${editingItem.uuid}`, payload)
      } else {
        await apiPost('/agents/subscriptions', payload)
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
    if (!(await confirm({ message: `Delete agent subscription "${item.host}:${item.port}" (${item.type})?`, variant: 'destructive' }))) return
    setError('')
    try {
      await apiDelete(`/agents/subscriptions/${item.uuid}`)
      await load()
    } catch (err) {
      setError(err.message)
    }
  }

  const formFields = (
    <>
      <Field label="Host">
        <Input
          value={form.host}
          onChange={(e) => setForm({ ...form, host: e.target.value })}
          placeholder="192.168.1.1"
        />
      </Field>
      <Field label="Port">
        <DigitsInput value={form.port} onValueChange={(port) => setForm({ ...form, port })} />
      </Field>
      <Field label="Protocol">
        <Select
          value={form.protocol}
          onValueChange={(v) => setForm({ ...form, protocol: v })}
        >
          <SelectTrigger className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="udp">UDP</SelectItem>
            <SelectItem value="tcp">TCP</SelectItem>
            <SelectItem value="tls">TLS</SelectItem>
          </SelectContent>
        </Select>
      </Field>
      <Field label="Path">
        <Input
          value={form.path}
          onChange={(e) => setForm({ ...form, path: e.target.value })}
          placeholder="/api"
        />
      </Field>
      <Field label="Node">
        <Input
          value={form.node}
          onChange={(e) => setForm({ ...form, node: e.target.value })}
          placeholder="node1"
        />
      </Field>
      <Field label="Type">
        {agentTypes.length > 0 ? (
          <Select
            value={form.type || undefined}
            onValueChange={(v) => setForm({ ...form, type: v })}
          >
            <SelectTrigger className="w-full">
              <SelectValue placeholder="Select type..." />
            </SelectTrigger>
            <SelectContent>
              {agentTypes.map((t) => (
                <SelectItem key={t} value={t}>
                  {t}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        ) : (
          <Input
            value={form.type}
            onChange={(e) => setForm({ ...form, type: e.target.value })}
            placeholder="home"
          />
        )}
      </Field>
      <Field label="TTL (seconds)">
        <DigitsInput value={form.ttl} onValueChange={(ttl) => setForm({ ...form, ttl })} />
      </Field>
      <Field label="Active">
        <Select
          value={String(form.active)}
          onValueChange={(v) => setForm({ ...form, active: Number(v) })}
        >
          <SelectTrigger className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="1">Active</SelectItem>
            <SelectItem value="0">Inactive</SelectItem>
          </SelectContent>
        </Select>
      </Field>
    </>
  )

  return (
    <CrudTable
      title="Agent Subscriptions"
      description="Register remote capture agents and route their traffic by node and type."
      columns={columns}
      items={items}
      loading={loading}
      onLoad={load}
      error={error}
      idField="uuid"
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
                  editingItem.type,
                  `${editingItem.host}:${editingItem.port}`,
                  'Edit Subscription',
                )
              : 'Edit Subscription'
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
          title="Create Agent Subscription"
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
            placeholder="Type"
            value={filters.type}
            onChange={(e) => setFilters({ ...filters, type: e.target.value })}
          />
          <Input
            placeholder="Node"
            value={filters.node}
            onChange={(e) => setFilters({ ...filters, node: e.target.value })}
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
