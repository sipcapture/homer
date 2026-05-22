// @ts-nocheck - TODO: rewrite with shadcn/ui + TanStack; typed when refactored
import { useCallback, useEffect, useState } from 'react'
import { Search, Eraser, Table, BarChart3, Settings, ChevronDown, Link2 } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import SqlEditor, {
  HOMER_SQL_WRAP_LINES_SYNC,
  loadSqlWrapLinesPreference,
  persistSqlWrapLinesPreference,
} from '@/components/ui/sql-editor'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Checkbox } from '@/components/ui/checkbox'
import { apiGet, apiPost } from '@/api'
import { useDashboard } from '../context/DashboardContext'
import { resolveTimeRange } from '../utils/resolveTimeRange'
import { useSearchStore } from '../stores/search-store'
import { getWidgetMeta } from './registry'
import { computeAvailableRows, fitWidgetHeight } from '../utils/grid-utils'
import SearchWidgetSettings from '../components/SearchWidgetSettings'
import MultiSelectInput from '../components/MultiSelectInput'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { getMergedFields, useMappings } from '@/hooks/useMappings'
import { buildSearchDeepLinkURL } from '../searchDeepLink'

const METHOD_OPTIONS = [
  ['', 'Any'], ['INVITE', 'INVITE'], ['ACK', 'ACK'], ['BYE', 'BYE'],
  ['CANCEL', 'CANCEL'], ['REGISTER', 'REGISTER'], ['OPTIONS', 'OPTIONS'],
]

const EVENT_OPTIONS = [
  ['call', 'Call'], ['registration', 'Registration'], ['default', 'Default'],
]

const PROTO_OPTIONS = [
  ['1', 'SIP (1)'], ['5', 'RTCP (5)'], ['34', 'RTP Agent (34)'],
  ['35', 'RTP Stats (35)'], ['53', 'DNS (53)'], ['100', 'LOG (100)'],
]

/** Same extras as SearchWidgetSettings — appended when mapping has no limit row. */
const DEFAULT_SEARCH_EXTRA_FIELDS = [
  { id: 'limit', name: 'Query Limit', type: 'integer', form_type: 'input', form_default: '100' },
  {
    id: 'results_container',
    name: 'Results Container',
    type: 'string',
    form_type: 'select',
    form_default: 'default',
  },
]

/** Visible SIP call fields to enable on a brand-new Protocol Search widget. */
const DEFAULT_PROTO_SELECTED_IDS = new Set([
  'call_id',
  'method',
  'from_user',
  'to_user',
  'src_ip',
  'dst_ip',
  'src_port',
  'dst_port',
  'response_code',
])

/**
 * SIP mapping presets (hepid/profile match mapping_seed.go).
 */
const SIP_PRESETS = {
  sip_call: {
    hepid: 1,
    profile: 'call',
    selected: DEFAULT_PROTO_SELECTED_IDS,
  },
  sip_registration: {
    hepid: 1,
    profile: 'registration',
    selected: new Set([
      'call_id',
      'method',
      'aor',
      'contact',
      'expires',
      'src_ip',
      'dst_ip',
      'src_port',
      'dst_port',
      'response_code',
    ]),
  },
}

/**
 * OTLP preset bootstrap configuration. Picked when a new Protocol Search
 * widget is added via the AddWidgetDialog with `config.preset` set
 * (see registry.ts → search.extras). The hepid/profile values mirror
 * the seeded mapping_schema rows in coordinator/services/mapping_seed.go.
 */
const OTLP_PRESETS = {
  otlp_traces: {
    hepid: 200,
    profile: 'default',
    selected: new Set(['trace_id', 'name', 'service_name', 'status_code']),
  },
  otlp_metrics: {
    hepid: 201,
    profile: 'default',
    selected: new Set(['name', 'type', 'service_name', 'value_double']),
  },
  otlp_logs: {
    hepid: 202,
    profile: 'default',
    selected: new Set(['severity_text', 'service_name', 'body', 'trace_id', 'span_id']),
  },
}

function findMappingByHepID(mappings, hepid, profile) {
  if (!Array.isArray(mappings) || mappings.length === 0) return null
  return (
    mappings.find(
      (m) => Number(m.hepid) === Number(hepid) && m.profile === profile,
    ) || null
  )
}

function pickDefaultSipCallMapping(mappings) {
  if (!Array.isArray(mappings) || mappings.length === 0) return null
  const sipCall = mappings.find((m) => Number(m.hepid) === 1 && m.profile === 'call')
  if (sipCall) return sipCall
  const sip = mappings.find((m) => Number(m.hepid) === 1)
  if (sip) return sip
  return mappings[0]
}

/**
 * Pick the mapping + default-selected field set when bootstrapping an empty
 * Protocol Search widget. Honours `config.preset` (from AddWidgetDialog
 * extras) before falling back to SIP/call. Preset keys: `sip_call`,
 * `sip_registration`, OTLP keys, or unset for default SIP/call.
 */
function pickBootstrapMapping(mappings, preset) {
  const sip = preset && SIP_PRESETS[preset]
  if (sip) {
    const m = findMappingByHepID(mappings, sip.hepid, sip.profile)
    if (m) return { mapping: m, selectedIds: sip.selected }
    return null
  }
  const otlp = preset && OTLP_PRESETS[preset]
  if (otlp) {
    const m = findMappingByHepID(mappings, otlp.hepid, otlp.profile)
    if (m) return { mapping: m, selectedIds: otlp.selected }
    // Preset requested but mapping_schema row missing — leave the widget
    // unbootstrapped so the user notices and reseeds the coordinator.
    return null
  }
  const m = pickDefaultSipCallMapping(mappings)
  if (!m) return null
  return { mapping: m, selectedIds: DEFAULT_PROTO_SELECTED_IDS }
}

/** One-time bootstrap per widget id (survives React StrictMode remount). */
const SEARCH_WIDGET_BOOTSTRAPPED_IDS = new Set()

/** Preset options for MultiSelectInput from mapping (selector or form_default). */
function multiSelectOptionsFromMappingField(field) {
  const { selector, form_default: fd } = field
  if (Array.isArray(selector) && selector.length > 0) {
    return selector.map((s) => ({
      name: String(s?.name ?? s?.value ?? ''),
      value: String(s?.value ?? s?.name ?? ''),
    }))
  }
  if (Array.isArray(fd) && fd.length > 0) {
    return fd.map((s) => {
      if (s && typeof s === 'object') {
        return {
          name: String(s.name ?? s.value ?? ''),
          value: String(s.value ?? s.name ?? ''),
        }
      }
      const t = String(s)
      return { name: t, value: t }
    })
  }
  return []
}

export default function SearchPanel({ config, onConfigChange, widgetId }) {
  const { publishSearch, widgets, updateWidgets, setLocked, locked, timeRange, timeZone, activeDashboardId } = useDashboard()
  // Never use '' — search-store ignores setField/setActiveTab when key is empty, so the form
  // would not persist (SQL / filters) and Search would appear to do nothing.
  const storeKey = `${activeDashboardId || '_'}:${widgetId ?? 'search'}`
  const { form, activeTab, limit } = useSearchStore((s) => s.getForm(storeKey))
  const { setField, setActiveTab, setLimit, clearForm } = useSearchStore.getState()

  const [showSettings, setShowSettings] = useState(false)
  const [sqlWrapLines, setSqlWrapLines] = useState(() => loadSqlWrapLinesPreference())
  const [otlpMetricNames, setOtlpMetricNames] = useState([])
  const [otlpMetricNamesLoading, setOtlpMetricNamesLoading] = useState(false)
  const [otlpMetricNamesErr, setOtlpMetricNamesErr] = useState('')
  /** Draft string while editing Limit so empty input does not coerce to 0 in the store */
  const [limitDraft, setLimitDraft] = useState<string | null>(null)

  const { mappings, loading: mappingsLoading } = useMappings()

  useEffect(() => {
    setLimitDraft(null)
  }, [storeKey])

  const limitInputValue = limitDraft !== null ? limitDraft : String(limit)

  const handleLimitInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const raw = e.target.value
    setLimitDraft(raw)
    const trimmed = raw.trim()
    if (trimmed === '') return
    const n = parseInt(trimmed, 10)
    if (!Number.isFinite(n) || n < 1 || n > 50000) return
    setLimit(storeKey, n)
  }

  const handleLimitInputBlur = () => {
    setLimitDraft(null)
  }

  // New Protocol Search widgets start with empty `fields` (or { preset } from
  // AddWidgetDialog extras) — bootstrap protocol / profile / fields once.
  useEffect(() => {
    const wid = String(widgetId ?? '')
    if (!wid) return
    if (SEARCH_WIDGET_BOOTSTRAPPED_IDS.has(wid)) return
    if (mappingsLoading) return
    if (config?.fields && config.fields.length > 0) {
      SEARCH_WIDGET_BOOTSTRAPPED_IDS.add(wid)
      return
    }
    const picked = pickBootstrapMapping(mappings, config?.preset)
    if (!picked) {
      if (mappings.length === 0) SEARCH_WIDGET_BOOTSTRAPPED_IDS.add(wid)
      return
    }
    const { mapping: m, selectedIds } = picked
    const merged = getMergedFields(m)
    const withExtras = merged.find((f) => f.id === 'limit')
      ? merged
      : [...merged, ...DEFAULT_SEARCH_EXTRA_FIELDS]
    const mapped = withExtras
      .filter((f) => !f.skip)
      .map((f) => {
        const selected =
          f.selected === true ||
          (!f.hide && selectedIds.has(f.id)) ||
          f.id === 'limit' ||
          f.id === 'results_container'
        return {
          ...f,
          selected,
        }
      })
    if (!mapped.some((f) => f.selected)) {
      // Fallback: enable the first sid_type / id-like column so the form
      // is not empty after bootstrap (mirrors the SIP call_id fallback).
      const fallbackId =
        mapped.find((f) => f.sid_type === true)?.id ??
        mapped.find((f) => f.id === 'call_id' || f.id === 'trace_id')?.id ??
        mapped[0]?.id
      if (fallbackId) {
        mapped.forEach((f) => {
          if (f.id === fallbackId) f.selected = true
        })
      }
    }
    onConfigChange?.({
      ...config,
      protocol_id: { name: m.hep_alias, value: m.hepid },
      protocol_profile: m.profile,
      fields: mapped.map((f) => ({
        ...f,
        proto: `${m.hep_alias}-${m.profile}`,
        field_name: f.id,
      })),
    })
    SEARCH_WIDGET_BOOTSTRAPPED_IDS.add(wid)
  }, [mappingsLoading, mappings, onConfigChange, widgetId, config?.fields, config?.preset])

  useEffect(() => {
    const onSync = (e) => {
      if (typeof e?.detail === 'boolean') setSqlWrapLines(e.detail)
    }
    window.addEventListener(HOMER_SQL_WRAP_LINES_SYNC, onSync)
    return () => window.removeEventListener(HOMER_SQL_WRAP_LINES_SYNC, onSync)
  }, [])

  /** Distinct OTLP metric names for the dashboard time window (preset otlp_metrics). */
  useEffect(() => {
    if (config?.preset !== 'otlp_metrics') return
    const ts = resolveTimeRange(timeRange, timeZone)
    if (!ts?.from || !ts?.to) return
    let cancelled = false
    const svc = String(form?.service_name ?? '').trim()
    const timer = window.setTimeout(async () => {
      setOtlpMetricNamesLoading(true)
      setOtlpMetricNamesErr('')
      try {
        const body = { timestamp: { from: ts.from, to: ts.to } }
        if (svc) body.service_name = svc
        const res = await apiPost('/transactions/otlp-metric-names', body)
        if (cancelled) return
        const items = Array.isArray(res?.data?.items) ? res.data.items : []
        const names = items
          .map((r) => String(r?.name ?? r?.NAME ?? '').trim())
          .filter(Boolean)
        setOtlpMetricNames([...new Set(names)])
      } catch (e) {
        if (!cancelled) {
          setOtlpMetricNamesErr(e?.message || 'Failed to load metric names')
          setOtlpMetricNames([])
        }
      } finally {
        if (!cancelled) setOtlpMetricNamesLoading(false)
      }
    }, 400)
    return () => {
      cancelled = true
      window.clearTimeout(timer)
    }
  }, [config?.preset, timeRange, timeZone, form?.service_name])
  const [llmStatus, setLlmStatus] = useState<{ enabled: boolean; model?: string; provider?: string; reachable?: boolean; key_configured?: boolean } | null>(null)

  // Lazy LLM-status probe: only fired the first time the AI tab is opened so
  // we don't pay for /api/v4/mcp/llm/status on dashboards that never use AI.
  useEffect(() => {
    if (activeTab !== 'ai' || llmStatus !== null) return
    let cancelled = false
    apiGet<{ data?: Record<string, unknown> }>('/mcp/llm/status', { check: false })
      .then((j) => {
        if (cancelled || !j?.data) return
        setLlmStatus({
          enabled: !!j.data.enabled,
          model: j.data.model as string | undefined,
          provider: j.data.provider as string | undefined,
          reachable: !!j.data.reachable,
          key_configured: !!j.data.key_configured,
        })
      })
      .catch(() => {
        if (!cancelled) setLlmStatus({ enabled: false })
      })
    return () => { cancelled = true }
  }, [activeTab, llmStatus])

  const savedTarget = config?.targetWidget || ''
  /** Ignore targetWidget pointing at a deleted or non-searchable widget (otherwise Search does nothing). */
  const targetId =
    savedTarget &&
    widgets.some(
      (w) => w.id === savedTarget && (w.type === 'results' || w.type === 'chart'),
    )
      ? savedTarget
      : ''

  useEffect(() => {
    const t = config?.targetWidget
    if (!t || !onConfigChange) return
    if (!widgets.some((w) => w.id === t)) {
      onConfigChange({ ...(config || {}), targetWidget: '' })
    }
  }, [config, widgets, onConfigChange])

  const configuredFields = config?.fields
  const isConfigured = Array.isArray(configuredFields) && configuredFields.length > 0

  const resultWidgets = widgets.filter((w) => w.type === 'results' || w.type === 'chart')

  const handleChange = useCallback((fieldName, value) => setField(storeKey, fieldName, value), [storeKey, setField])

  const buildTimestamp = () => {
    const ts = resolveTimeRange(timeRange, timeZone)
    return ts || {}
  }

  const handleSearch = () => {
    let filter

    if (isConfigured) {
      // Dynamic mode: build filter from configured fields
      const protoType = config.protocol_id?.value ?? 0
      const eventType = config.protocol_profile ?? ''
      filter = { proto_type: Number(protoType), event_type: eventType }
      configuredFields.forEach((f) => {
        // Virtual absent/present: checkbox only — handle before empty-value skip
        if (f.virtual && typeof f.virtual === 'object' && f.virtual.kind) {
          const vm = String(f.virtual.match || 'like').toLowerCase()
          if (vm === 'absent' || vm === 'present') {
            const checked = form[f.id] === true || form[f.id] === '1'
            if (!checked) return
            if (vm === 'absent') {
              if (!filter.virtual_absent) filter.virtual_absent = []
              filter.virtual_absent.push(f.id)
            } else {
              if (!filter.virtual_present) filter.virtual_present = []
              filter.virtual_present.push(f.id)
            }
            return
          }
        }

        const val = form[f.id]
        if (val === undefined || val === null || val === '') return

        // Virtual string fields (like / equals): values go under filter.virtual[id]
        if (f.virtual && typeof f.virtual === 'object' && f.virtual.kind) {
          if (!filter.virtual) filter.virtual = {}
          if (Array.isArray(val)) {
            if (val.length === 0) return
            filter.virtual[f.id] = val.length === 1 ? val[0] : val.join(',')
          } else {
            filter.virtual[f.id] = val
          }
          return
        }

        // Handle multiselect arrays
        if (Array.isArray(val)) {
          if (val.length === 0) return
          if (val.length === 1) {
            // Single value: use singular key for cleaner SQL
            filter[f.id] = val[0]
          } else {
            // Multiple values: use plural key for IN (...) query
            filter[f.id + 's'] = val
          }
          return
        }

        if (f.type === 'integer' || f.type === 'int') {
          const n = Number(val)
          if (!isNaN(n) && n > 0) filter[f.id] = n
        } else {
          filter[f.id] = val
        }
      })
    } else {
      // Legacy static mode
      const protoType = form.proto_type ? Number(form.proto_type) : 0
      filter = { proto_type: isNaN(protoType) ? 0 : protoType, event_type: form.event_type }
      if (form.method) filter.method = form.method
      if (form.call_id) filter.call_id = form.call_id
      if (form.from_user) filter.from_user = form.from_user
      if (form.to_user) filter.to_user = form.to_user
      if (form.ruri_user) filter.ruri_user = form.ruri_user
      if (form.user_agent) filter.user_agent = form.user_agent
      if (form.src_ip) filter.src_ip = form.src_ip
      if (form.dst_ip) filter.dst_ip = form.dst_ip
      if (Number(form.src_port) > 0) filter.src_port = Number(form.src_port)
      if (Number(form.dst_port) > 0) filter.dst_port = Number(form.dst_port)
      if (Number(form.capture_id) > 0) filter.capture_id = Number(form.capture_id)
      if (form.node) filter.node = form.node
    }

    const searchData = {
      filter,
      param: { limit: Number(limit) || 50 },
      // AI / MCP: do not send the dashboard time window — the server infers
      // range from natural language (and LLM fallbacks). Sending the previous
      // range here pinned "last two hours" to the old 1h window after the
      // first query had already synced the picker.
      timestamp: activeTab === 'ai' ? {} : buildTimestamp(),
      sql: activeTab === 'sql' ? form.sql : undefined,
      useSqlEndpoint: activeTab === 'sql' && form.sql?.trim(),
      nl_query: activeTab === 'ai' ? form.nl_query : undefined,
      nl_mode: activeTab === 'ai' ? (form.nl_mode || 'auto') : undefined,
      nl_parser: activeTab === 'ai' ? (form.nl_parser || 'auto') : undefined,
      useMcpEndpoint: activeTab === 'ai' && form.nl_query?.trim(),
    }

    if (targetId) {
      publishSearch(targetId, searchData)
    } else {
      resultWidgets.forEach((w) => publishSearch(w.id, searchData))
    }
  }

  const handleClear = () => clearForm(storeKey)

  const handleCopySearchLink = async () => {
    const ts = resolveTimeRange(timeRange, timeZone) || {}
    const url = buildSearchDeepLinkURL(
      { origin: window.location.origin, pathname: window.location.pathname },
      {
        from_user: form.from_user,
        to_user: form.to_user,
        call_id: form.call_id,
        method: Array.isArray(form.method) ? form.method[0] : form.method,
        src_ip: form.src_ip,
        dst_ip: form.dst_ip,
        proto_type: form.proto_type,
        event_type: form.event_type,
        limit,
      },
      {
        from: ts.from ?? Date.now() - 3600000,
        to: ts.to ?? Date.now(),
      },
    )
    try {
      await navigator.clipboard.writeText(url)
      toast.success('Search link copied')
    } catch {
      toast.error('Could not copy link')
    }
  }

  const handleSettingsSave = (newConfig) => {
    onConfigChange?.({ ...config, ...newConfig })
  }

  // ── Field renderers ──────────────────────────────────────────────────────────

  const textField = (id, label, type = 'text', placeholder = '') => (
    <div className="grid gap-1" key={id}>
      <Label htmlFor={`sp-${id}`} className="text-[11px] text-muted-foreground">{label}</Label>
      <Input
        id={`sp-${id}`}
        type={type}
        placeholder={placeholder}
        value={form[id] || ''}
        onChange={(e) => handleChange(id, e.target.value)}
        className="h-7 text-xs"
      />
    </div>
  )

  const selectField = (id, label, options) => (
    <div className="grid gap-1" key={id}>
      <Label htmlFor={`sp-${id}`} className="text-[11px] text-muted-foreground">{label}</Label>
      <Select
        value={form[id] || '__any'}
        onValueChange={(v) => handleChange(id, v === '__any' ? '' : v)}
      >
        <SelectTrigger id={`sp-${id}`} className="h-7 text-xs">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {options.map(([v, l]) => (
            <SelectItem key={v || '__any'} value={v || '__any'}>{l}</SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )

  // Render a single dynamic field from fields_mapping metadata
  const renderDynamicField = (field) => {
    const { id, name, form_type, selector, type } = field
    const vMatch = field.virtual?.match != null ? String(field.virtual.match).toLowerCase() : ''
    if (field.virtual?.kind && (vMatch === 'absent' || vMatch === 'present')) {
      const checked = form[id] === true || form[id] === '1'
      return (
        <div className="flex items-center gap-2 py-0.5" key={id}>
          <Checkbox
            id={`sp-${id}`}
            checked={checked}
            onCheckedChange={(v) => handleChange(id, v === true)}
          />
          <Label htmlFor={`sp-${id}`} className="cursor-pointer text-[11px] text-muted-foreground">
            {name}
          </Label>
        </div>
      )
    }
    if (config?.preset === 'otlp_metrics' && id === 'name') {
      return (
        <div className="grid gap-1" key={id}>
          <Label htmlFor={`sp-${id}`} className="text-[11px] text-muted-foreground">{name}</Label>
          {otlpMetricNamesLoading ? (
            <span className="text-[10px] text-muted-foreground">Loading metric names…</span>
          ) : null}
          {otlpMetricNamesErr ? (
            <span className="text-[10px] text-destructive">{otlpMetricNamesErr}</span>
          ) : null}
          <div className="flex gap-1">
            <Input
              id={`sp-${id}`}
              value={form[id] || ''}
              onChange={(e) => handleChange(id, e.target.value)}
              placeholder="Type name or pick from list"
              className="h-7 min-w-0 flex-1 font-mono text-xs"
              autoComplete="off"
              spellCheck={false}
            />
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="h-7 shrink-0 px-2"
                  disabled={otlpMetricNamesLoading || otlpMetricNames.length === 0}
                  title="Choose metric name from names loaded for this time range"
                  aria-label="Choose metric name from list"
                >
                  <ChevronDown className="h-3.5 w-3.5" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent
                align="end"
                className="max-h-64 min-w-[min(90vw,22rem)] overflow-y-auto"
              >
                {otlpMetricNames.map((n) => (
                  <DropdownMenuItem
                    key={n}
                    className="cursor-pointer font-mono text-xs"
                    onSelect={() => handleChange(id, n)}
                  >
                    {n}
                  </DropdownMenuItem>
                ))}
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
          {!otlpMetricNamesLoading && !otlpMetricNamesErr && otlpMetricNames.length === 0 ? (
            <p className="text-[10px] text-muted-foreground">No names in this time window — widen the range or check ingest.</p>
          ) : null}
        </div>
      )
    }
    if (id === 'limit') {
      return (
        <div className="grid gap-1" key={id}>
          <Label htmlFor="sp-limit" className="text-[11px] text-muted-foreground">Limit</Label>
          <Input
            id="sp-limit"
            type="number"
            inputMode="numeric"
            value={limitInputValue}
            min={1}
            max={50000}
            onChange={handleLimitInputChange}
            onBlur={handleLimitInputBlur}
            className="h-7 text-xs"
          />
        </div>
      )
    }
    if (id === 'results_container') return null // handled by target widget selector

    // multiselect (selector / form_default) or input_multi_select (form_default) → chips + custom
    if (form_type === 'multiselect' || form_type === 'input_multi_select') {
      const options = multiSelectOptionsFromMappingField(field)
      if (form_type === 'input_multi_select' || options.length > 0) {
        const selected = Array.isArray(form[id])
          ? (form[id] as string[])
          : form[id]
            ? [String(form[id])]
            : []
        return (
          <div className="grid gap-1" key={id}>
            <Label className="text-[11px] text-muted-foreground">{name}</Label>
            <MultiSelectInput
              id={`sp-${id}`}
              options={options}
              value={selected}
              onChange={(vals) => handleChange(id, vals)}
              placeholder="Any"
            />
          </div>
        )
      }
    }

    if (form_type === 'select' && Array.isArray(selector) && selector.length > 0) {
      const options = [['', 'Any'], ...selector.map((s) => [s.value, s.name])]
      return selectField(id, name, options)
    }
    if (form_type === 'loki-field') {
      return (
        <div className="grid gap-1" key={id}>
          <Label htmlFor={`sp-${id}`} className="text-[11px] text-muted-foreground">{name}</Label>
          <Textarea
            id={`sp-${id}`}
            value={form[id] || ''}
            onChange={(e) => handleChange(id, e.target.value)}
            placeholder="LogQL query"
            rows={3}
            className="font-mono text-xs"
          />
        </div>
      )
    }
    if (form_type === 'datetime') {
      return textField(id, name, 'datetime-local')
    }
    // default: text input
    const inputType = (type === 'integer' || type === 'int') ? 'number' : 'text'
    return textField(id, name, inputType)
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
      e.preventDefault()
      handleSearch()
    }
  }

  return (
    <div className="flex h-full min-h-0 flex-col p-2" onKeyDown={handleKeyDown}>
      <Tabs value={activeTab} onValueChange={(tab) => setActiveTab(storeKey, tab)} className="min-h-0 flex-1">
        <div className="flex items-center gap-1">
          <TabsList className="flex-1">
            <TabsTrigger value="form">Form</TabsTrigger>
            <TabsTrigger value="sql">SQL</TabsTrigger>
            <TabsTrigger value="ai">AI</TabsTrigger>
          </TabsList>
          <Button
            size="icon-xs"
            variant="ghost"
            onClick={() => setShowSettings(true)}
            title="Widget settings"
            className="shrink-0"
          >
            <Settings className="h-3.5 w-3.5" />
          </Button>
        </div>

        <TabsContent value="form" className="mt-2 min-h-0 overflow-y-auto">
          <div className="flex flex-col gap-2 pr-1">
            {isConfigured && config?.preset === 'otlp_metrics' ? (
              <p className="text-[10px] text-muted-foreground">
                Choose a metric name, then Search. Add a Time Chart widget and set Target to that chart to plot values over time (bucket / Y mode in chart settings).
              </p>
            ) : null}
            {isConfigured
              ? configuredFields.map((f) => renderDynamicField(f)).filter(Boolean)
              : (
                <>
                  {textField('call_id', 'Call-ID', 'text', 'SIP Call-ID')}
                  {textField('from_user', 'From', 'text', 'Caller')}
                  {textField('to_user', 'To', 'text', 'Callee')}
                  {selectField('method', 'Method', METHOD_OPTIONS)}
                  {selectField('event_type', 'Event', EVENT_OPTIONS)}
                  {selectField('proto_type', 'Protocol', PROTO_OPTIONS)}
                  {textField('src_ip', 'Src IP', 'text', '10.0.0.1')}
                  {textField('dst_ip', 'Dst IP', 'text', '10.0.0.2')}
                  {textField('src_port', 'Src Port', 'number', '5060')}
                  {textField('dst_port', 'Dst Port', 'number', '5060')}
                  {textField('ruri_user', 'R-URI', 'text', 'R-URI user')}
                  {textField('user_agent', 'UA', 'text', 'User Agent')}
                  {textField('node', 'Node', 'text', 'Node ID')}
                  <div className="grid gap-1">
                    <Label htmlFor="sp-limit" className="text-[11px] text-muted-foreground">Limit</Label>
                    <Input
                      id="sp-limit"
                      type="number"
                      inputMode="numeric"
                      value={limitInputValue}
                      min={1}
                      max={50000}
                      onChange={handleLimitInputChange}
                      onBlur={handleLimitInputBlur}
                      className="h-7 text-xs"
                    />
                  </div>
                </>
              )}
          </div>
        </TabsContent>

        <TabsContent value="sql" className="mt-2 min-h-0 overflow-y-auto">
          <div className="flex flex-col gap-2">
            <div className="flex items-center gap-2">
              <Checkbox
                id="sp-sql-wrap"
                checked={sqlWrapLines}
                onCheckedChange={(c) => {
                  const v = c === true
                  setSqlWrapLines(v)
                  persistSqlWrapLinesPreference(v)
                }}
              />
              <Label htmlFor="sp-sql-wrap" className="cursor-pointer text-[11px] font-normal text-muted-foreground">
                Wrap long lines
              </Label>
            </div>
            <div className="grid min-h-0 gap-1">
              <Label htmlFor="sp-sql" className="text-[11px] text-muted-foreground">
                SQL Query
                <span className="ml-1 font-normal text-muted-foreground/80">(Ctrl+Enter to search)</span>
              </Label>
              <SqlEditor
                id="sp-sql"
                value={form.sql || ''}
                onChange={(v) => handleChange('sql', v)}
                onSubmit={handleSearch}
                minHeight={240}
                wrapLines={sqlWrapLines}
                placeholder="SELECT uuid, method, timestamp FROM homer_lake.main.hep_proto_1_call LIMIT 100"
                className="min-h-[240px] font-mono"
              />
            </div>
          </div>
        </TabsContent>

        <TabsContent value="ai" className="mt-2 min-h-0 overflow-y-auto">
          <div className="flex flex-col gap-2">
            {llmStatus && (
              <div className="flex items-center gap-1.5 rounded-sm border border-border bg-muted/30 px-2 py-1 text-[10px] text-muted-foreground">
                <span
                  className={`inline-block h-1.5 w-1.5 rounded-full ${
                    llmStatus.enabled ? 'bg-emerald-500' : 'bg-zinc-500'
                  }`}
                  title={llmStatus.enabled ? 'LLM bridge enabled' : 'LLM bridge disabled (regex only)'}
                />
                {llmStatus.enabled
                  ? `LLM: ${llmStatus.provider || 'openai'} · ${llmStatus.model || '?'}${llmStatus.key_configured ? '' : ' · no key'}`
                  : 'LLM disabled — regex parser only'}
              </div>
            )}
            <div className="grid gap-1">
              <Label htmlFor="sp-nl" className="text-[11px] text-muted-foreground">Natural Language Query</Label>
              <Textarea
                id="sp-nl"
                value={form.nl_query}
                onChange={(e) => handleChange('nl_query', e.target.value)}
                placeholder="Find all INVITE messages from the last hour"
                rows={5}
                className="text-xs"
              />
            </div>
            <div className="grid grid-cols-2 gap-2">
              <div className="grid gap-1">
                <Label htmlFor="sp-nl-mode" className="text-[11px] text-muted-foreground">Mode</Label>
                <Select value={form.nl_mode || 'auto'} onValueChange={(v) => handleChange('nl_mode', v)}>
                  <SelectTrigger id="sp-nl-mode" className="h-7 text-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="auto">Auto</SelectItem>
                    <SelectItem value="structured">Structured</SelectItem>
                    <SelectItem value="sql">SQL</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-1">
                <Label htmlFor="sp-nl-parser" className="text-[11px] text-muted-foreground">Parser</Label>
                <Select
                  value={form.nl_parser || 'auto'}
                  onValueChange={(v) => handleChange('nl_parser', v)}
                >
                  <SelectTrigger id="sp-nl-parser" className="h-7 text-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="auto">Auto (LLM → regex)</SelectItem>
                    <SelectItem
                      value="llm"
                      disabled={!!llmStatus && !llmStatus.enabled}
                    >
                      LLM only
                    </SelectItem>
                    <SelectItem value="regex">Regex only</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>
            <div className="grid gap-1">
              <Label htmlFor="sp-ai-limit" className="text-[11px] text-muted-foreground">Limit</Label>
              <Input
                id="sp-ai-limit"
                type="number"
                inputMode="numeric"
                value={limitInputValue}
                min={1}
                max={50000}
                onChange={handleLimitInputChange}
                onBlur={handleLimitInputBlur}
                className="h-7 text-xs"
              />
            </div>
          </div>
        </TabsContent>
      </Tabs>

      {resultWidgets.length > 1 && (
        <div className="grid shrink-0 gap-1 px-1 pt-2">
          <Label htmlFor="sp-target" className="text-[11px] text-muted-foreground">Target</Label>
          <Select
            value={targetId || '__all'}
            onValueChange={(v) => onConfigChange?.({ ...config, targetWidget: v === '__all' ? '' : v })}
          >
            <SelectTrigger id="sp-target" className="h-7 text-xs">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="__all">All results / charts</SelectItem>
              {resultWidgets.map((w) => (
                <SelectItem key={w.id} value={w.id}>
                  {w.title || w.type} ({w.id.slice(-6)})
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      )}

      {resultWidgets.length === 0 && (
        <div className="shrink-0 rounded-sm border border-dashed border-amber-500/40 bg-amber-500/5 p-2">
          <p className="mb-2 text-[11px] text-amber-400">
            No results or Time Chart widget yet — add one to see search output.
          </p>
          <div className="flex gap-1.5">
            <Button
              size="sm"
              variant="outline"
              className="h-6 flex-1 text-[10px]"
              onClick={() => {
                const meta = getWidgetMeta('results')!
                const rows = computeAvailableRows(window.innerHeight - 112)
                if (locked) setLocked(false)
                updateWidgets([...widgets, { id: `results-${Date.now()}`, type: 'results', title: 'Results Table', x: 3, y: 0, w: meta.defaultW, h: fitWidgetHeight(meta.defaultH, meta.minH, rows) }])
              }}
            >
              <Table className="mr-1 h-3 w-3" />
              Results Table
            </Button>
            <Button
              size="sm"
              variant="outline"
              className="h-6 flex-1 text-[10px]"
              onClick={() => {
                const meta = getWidgetMeta('chart')!
                const rows = computeAvailableRows(window.innerHeight - 112)
                if (locked) setLocked(false)
                updateWidgets([...widgets, { id: `chart-${Date.now()}`, type: 'chart', title: 'Time Chart', x: 3, y: 0, w: meta.defaultW, h: fitWidgetHeight(meta.defaultH, meta.minH, rows) }])
              }}
            >
              <BarChart3 className="mr-1 h-3 w-3" />
              Time Chart
            </Button>
          </div>
        </div>
      )}

      <div className="flex shrink-0 items-center gap-2 border-t border-border pt-2">
        <Button size="sm" onClick={handleSearch} className="flex-1" title="Search (Ctrl+Enter)">
          <Search className="mr-1 h-4 w-4" />
          Search
        </Button>
        <Button size="sm" variant="outline" onClick={handleClear}>
          <Eraser className="mr-1 h-4 w-4" />
          Clear
        </Button>
        <Button
          size="sm"
          variant="outline"
          onClick={handleCopySearchLink}
          title="Copy URL with current filters (opens dashboard search)"
        >
          <Link2 className="h-4 w-4" />
        </Button>
      </div>

      <SearchWidgetSettings
        open={showSettings}
        onClose={() => setShowSettings(false)}
        config={config || {}}
        onSave={handleSettingsSave}
      />
    </div>
  )
}
