import { create } from 'zustand'
import { persist } from 'zustand/middleware'

// Field values can be a plain string or an array of strings (for multiselect fields)
export type FieldValue = string | string[]

export interface SearchFormFields {
  call_id: string
  from_user: string
  to_user: string
  method: FieldValue
  response_code: FieldValue
  src_ip: string
  dst_ip: string
  proto_type: string
  event_type: string
  node: string
  src_port: string
  dst_port: string
  ruri_user: string
  user_agent: string
  capture_id: string
  sql: string
  nl_query: string
  nl_mode: string
  [key: string]: FieldValue
}

export interface SearchFormState {
  form: SearchFormFields
  activeTab: string
  limit: number
}

const DEFAULT_FORM: SearchFormFields = {
  call_id: '',
  from_user: '',
  to_user: '',
  method: [],
  response_code: [],
  src_ip: '',
  dst_ip: '',
  proto_type: '1',
  event_type: 'call',
  node: '',
  src_port: '',
  dst_port: '',
  ruri_user: '',
  user_agent: '',
  capture_id: '',
  sql: '',
  nl_query: '',
  nl_mode: 'auto',
}

const DEFAULT_STATE: SearchFormState = {
  form: DEFAULT_FORM,
  activeTab: 'form',
  limit: 50,
}

interface SearchStore {
  forms: Record<string, SearchFormState>
  getForm: (key: string) => SearchFormState
  setField: (key: string, field: string, value: FieldValue) => void
  setActiveTab: (key: string, tab: string) => void
  setLimit: (key: string, limit: number) => void
  clearForm: (key: string) => void
}

function sanitizeFormsLimits(
  forms: Record<string, SearchFormState> | undefined,
): Record<string, SearchFormState> | undefined {
  if (!forms) return forms
  let changed = false
  const next = { ...forms }
  for (const k of Object.keys(next)) {
    const entry = next[k]
    const L = entry?.limit
    if (
      typeof L !== 'number' ||
      !Number.isFinite(L) ||
      L < 1 ||
      L > 50000
    ) {
      next[k] = { ...entry, limit: DEFAULT_STATE.limit }
      changed = true
    }
  }
  return changed ? next : forms
}

export const useSearchStore = create<SearchStore>()(
  persist(
    (set, get) => ({
      forms: {},

      getForm: (key: string): SearchFormState => {
        if (!key) return DEFAULT_STATE
        return get().forms[key] ?? DEFAULT_STATE
      },

      setField: (key, field, value: FieldValue) => {
        if (!key) return
        set((state) => {
          const prev = state.forms[key] ?? DEFAULT_STATE
          return {
            forms: {
              ...state.forms,
              [key]: {
                ...prev,
                form: { ...prev.form, [field]: value },
              },
            },
          }
        })
      },

      setActiveTab: (key, tab) => {
        if (!key) return
        set((state) => {
          const prev = state.forms[key] ?? DEFAULT_STATE
          return {
            forms: { ...state.forms, [key]: { ...prev, activeTab: tab } },
          }
        })
      },

      setLimit: (key, limit) => {
        if (!key) return
        const n = Number(limit)
        const safe =
          Number.isFinite(n) && n >= 1 && n <= 50000
            ? Math.floor(n)
            : DEFAULT_STATE.limit
        set((state) => {
          const prev = state.forms[key] ?? DEFAULT_STATE
          return {
            forms: { ...state.forms, [key]: { ...prev, limit: safe } },
          }
        })
      },

      clearForm: (key) => {
        if (!key) return
        set((state) => ({
          forms: { ...state.forms, [key]: DEFAULT_STATE },
        }))
      },
    }),
    {
      name: 'homer-search-forms',
      version: 1,
      migrate: (persistedState: unknown, version: number) => {
        const p = persistedState as { forms?: Record<string, SearchFormState> } | null
        if (!p?.forms) return persistedState as SearchStore
        const forms = sanitizeFormsLimits(p.forms)
        return forms === p.forms ? (persistedState as SearchStore) : { ...p, forms: forms! }
      },
      merge: (persistedState, currentState) => {
        const p = persistedState as Partial<SearchStore> | null | undefined
        const merged: SearchStore = {
          ...currentState,
          ...(p || {}),
          forms: sanitizeFormsLimits(p?.forms) ?? currentState.forms,
        }
        return merged
      },
    },
  ),
)
