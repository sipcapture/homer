import { useEffect, useMemo, useRef, useState } from 'react'
import CodeMirror from '@uiw/react-codemirror'
import { sql, StandardSQL } from '@codemirror/lang-sql'
import { lakeSqlColumnCompletion } from '@/components/ui/sql-lake-column-completion'
import { keymap, EditorView } from '@codemirror/view'
import { useTheme } from '@/components/theme/theme-provider'
import { cn } from '@/lib/utils'
import { apiGet } from '@/api'

const LS_KEY_SQL_WRAP = 'homer_sql_wrap_lines'

/** Same-tab sync between Search and Smart Input when wrap preference changes. */
export const HOMER_SQL_WRAP_LINES_SYNC = 'homer-sql-wrap-lines-change'

/** Shared preference for Search + Smart Input SQL editors (default: wrap on). */
export function loadSqlWrapLinesPreference(): boolean {
  if (typeof window === 'undefined') return true
  try {
    const v = window.localStorage.getItem(LS_KEY_SQL_WRAP)
    if (v === '0') return false
    return true
  } catch {
    return true
  }
}

export function persistSqlWrapLinesPreference(wrap: boolean) {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(LS_KEY_SQL_WRAP, wrap ? '1' : '0')
  } catch {
    /* ignore */
  }
  window.dispatchEvent(new CustomEvent(HOMER_SQL_WRAP_LINES_SYNC, { detail: wrap }))
}

/** Common HEP / DuckLake column names for SIP call tables (fallback hints). */
const HEP_SIP_CALL_COLUMNS = [
  'id',
  'timestamp',
  'ts',
  'create_date',
  'method',
  'call_id',
  'session_id',
  'correlation_id',
  'from_user',
  'to_user',
  'ruri_user',
  'user_agent',
  'src_ip',
  'dst_ip',
  'src_port',
  'dst_port',
  'protocol',
  'event',
  'capture_id',
  'node',
  'raw',
  'message',
] as const

const homerLakeTables = {
  hep_proto_1_call: [...HEP_SIP_CALL_COLUMNS],
  hep_proto_1_registration: [
    'timestamp',
    'method',
    'call_id',
    'src_ip',
    'dst_ip',
    'from_user',
    'to_user',
    'user_agent',
    'capture_id',
    'node',
  ],
  hep_proto_1_default: [...HEP_SIP_CALL_COLUMNS],
  hep_proto_5_default: ['timestamp', 'session_id', 'src_ip', 'dst_ip', 'capture_id', 'node'],
  hep_proto_100_default: ['timestamp', 'capture_id', 'node', 'message'],
} as const

/** Fallback nested schema: catalog → schema → table → columns (FQ names like homer_lake.main.hep_proto_1_call). */
const FALLBACK_SQL_SCHEMA: Record<string, Record<string, Record<string, string[]>>> = {
  homer_lake: {
    main: {
      ...Object.fromEntries(
        Object.entries(homerLakeTables).map(([k, v]) => [k, [...v]]),
      ),
    },
  },
  homer_lake_hot: {
    main: {
      ...Object.fromEntries(
        Object.entries(homerLakeTables).map(([k, v]) => [k, [...v]]),
      ),
    },
  },
  homer_lake_cold: {
    main: {
      ...Object.fromEntries(
        Object.entries(homerLakeTables).map(([k, v]) => [k, [...v]]),
      ),
    },
  },
}

/** Deep clone + overlay server columns (new tables/columns from the node appear in autocomplete). */
function mergeSqlAutocompleteSchema(
  fallback: Record<string, Record<string, Record<string, string[]>>>,
  server: Record<string, Record<string, Record<string, string[]>>> | null,
): Record<string, Record<string, Record<string, string[]>>> {
  const base = JSON.parse(JSON.stringify(fallback)) as Record<
    string,
    Record<string, Record<string, string[]>>
  >
  if (!server) return base
  for (const [catalog, schemas] of Object.entries(server)) {
    if (!schemas || typeof schemas !== 'object') continue
    if (!base[catalog]) base[catalog] = {}
    for (const [schemaName, tables] of Object.entries(schemas)) {
      if (!tables || typeof tables !== 'object') continue
      if (!base[catalog][schemaName]) base[catalog][schemaName] = {}
      for (const [tableName, cols] of Object.entries(tables)) {
        if (Array.isArray(cols) && cols.length > 0) {
          base[catalog][schemaName][tableName] = cols.map(String)
        }
      }
    }
  }
  return base
}

type Props = {
  value: string
  onChange: (value: string) => void
  id?: string
  minHeight?: number
  height?: string
  readOnly?: boolean
  className?: string
  placeholder?: string
  /** Fired on Ctrl+Enter / Cmd+Enter (e.g. run search). */
  onSubmit?: () => void
  /**
   * When true (default), load catalog/schema/table/column names from
   * GET /api/v4/db/sql-autocomplete-schema and merge with built-in hints so new
   * tables on the server get autocomplete (refreshed periodically).
   */
  liveSchema?: boolean
  /** When true (default), long lines wrap instead of horizontal scrolling (CodeMirror lineWrapping). */
  wrapLines?: boolean
}

export default function SqlEditor({
  value,
  onChange,
  id,
  minHeight = 220,
  height,
  readOnly = false,
  className,
  placeholder,
  onSubmit,
  liveSchema = true,
  wrapLines = true,
}: Props) {
  const { theme } = useTheme()
  const [serverSchema, setServerSchema] = useState<Record<
    string,
    Record<string, Record<string, string[]>>
  > | null>(null)

  useEffect(() => {
    if (!liveSchema) return
    let cancelled = false
    const load = () => {
      apiGet<{ data?: Record<string, Record<string, Record<string, string[]>>> }>(
        '/db/sql-autocomplete-schema',
      )
        .then((r) => {
          if (cancelled || !r?.data || typeof r.data !== 'object') return
          setServerSchema(r.data)
        })
        .catch(() => {
          /* keep fallback schema */
        })
    }
    load()
    const interval = window.setInterval(load, 90_000)
    return () => {
      cancelled = true
      window.clearInterval(interval)
    }
  }, [liveSchema])

  const mergedSchema = useMemo(
    () => mergeSqlAutocompleteSchema(FALLBACK_SQL_SCHEMA, serverSchema),
    [serverSchema],
  )

  const resolvedTheme = useMemo<'dark' | 'light'>(() => {
    if (theme === 'dark') return 'dark'
    if (theme === 'light') return 'light'
    return window.matchMedia('(prefers-color-scheme: dark)').matches
      ? 'dark'
      : 'light'
  }, [theme])

  const onSubmitRef = useRef(onSubmit)
  onSubmitRef.current = onSubmit

  const extensions = useMemo(() => {
    const exts = [
      sql({
        dialect: StandardSQL,
        schema: mergedSchema,
      }),
      // Fix column completion after catalog.schema.table. when Lezer parses the
      // table reference as CompositeIdentifier (built-in schema parents stay empty).
      StandardSQL.language.data.of({
        autocomplete: lakeSqlColumnCompletion(mergedSchema),
      }),
      EditorView.theme({
        '.cm-scroller': {
          overflowY: 'auto',
          overflowX: wrapLines ? 'hidden' : 'auto',
        },
      }),
      keymap.of([
        {
          key: 'Mod-Enter',
          run: () => {
            onSubmitRef.current?.()
            return true
          },
        },
      ]),
    ]
    if (wrapLines) exts.push(EditorView.lineWrapping)
    return exts
  }, [mergedSchema, wrapLines])

  return (
    <div
      id={id}
      className={cn(
        'flex min-h-0 flex-1 flex-col overflow-hidden rounded-md border border-input bg-background text-xs [&_.cm-editor]:min-h-0',
        className,
      )}
    >
      <CodeMirror
        value={value}
        theme={resolvedTheme}
        height={height ?? `${minHeight}px`}
        extensions={extensions}
        editable={!readOnly}
        readOnly={readOnly}
        placeholder={placeholder}
        basicSetup={{
          lineNumbers: true,
          highlightActiveLine: !readOnly,
          foldGutter: true,
          autocompletion: true,
          bracketMatching: true,
          closeBrackets: true,
          indentOnInput: true,
        }}
        onChange={(v) => onChange(v)}
      />
    </div>
  )
}
