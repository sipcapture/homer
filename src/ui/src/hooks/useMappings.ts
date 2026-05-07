import { useState, useEffect, useCallback } from 'react'
import { apiGet, apiPut, apiDelete } from '@/api'

export interface FieldItem {
  id: string
  name: string
  type: string
  form_type: 'input' | 'select' | 'multiselect' | 'input_multi_select' | 'datetime' | 'loki-field' | string
  form_default?: string | Array<{ name?: string; value?: string }>
  form_api?: string
  selector?: Array<{ name: string; value: string }>
  selected?: boolean
  skip?: boolean
  hide?: boolean
  category?: string
  index?: string
  position?: number
  proto?: string
}

export interface MappingItem {
  guid: string
  hepid: number
  hep_alias: string
  profile: string
  partid?: number
  version?: number
  retention?: number
  partition_step?: number
  fields_mapping?: FieldItem[]
  user_mapping?: FieldItem[]
  fields_settings?: unknown
  schema_mapping?: unknown
  schema_settings?: unknown
}

interface MappingsMergedResponse {
  data: {
    items: MappingItem[]
  }
  meta?: unknown
}

// Module-level cache so mappings are fetched once per session
let cachedMappings: MappingItem[] | null = null
let fetchPromise: Promise<MappingItem[]> | null = null

function parseMappingFields(raw: unknown): FieldItem[] {
  if (!raw) return []
  if (Array.isArray(raw)) return raw as FieldItem[]
  if (typeof raw === 'object' && raw !== null && 'data' in (raw as object)) {
    const obj = raw as { data: unknown }
    if (Array.isArray(obj.data)) return obj.data as FieldItem[]
  }
  return []
}

async function fetchMappings(): Promise<MappingItem[]> {
  const resp = await apiGet<MappingsMergedResponse>('/mappings/merged')
  const items = resp?.data?.items ?? []
  return items.map((item) => ({
    ...item,
    fields_mapping: parseMappingFields(item.fields_mapping),
    user_mapping: item.user_mapping ? parseMappingFields(item.user_mapping) : undefined,
  }))
}

export function getMergedFields(mapping: MappingItem): FieldItem[] {
  const base = mapping.fields_mapping ?? []
  const user = mapping.user_mapping
  if (!user || user.length === 0) return base

  // User preferences control only selected/position.
  // form_type, selector, name, type always come from the base mapping so that
  // server-side updates to field definitions (e.g. adding select dropdowns) are
  // always reflected regardless of previously saved user preferences.
  const userMap = new Map(user.map((u) => [u.id, u]))
  const merged = base.map((b) => {
    const override = userMap.get(b.id)
    if (!override) return b
    return {
      ...b,
      selected: override.selected ?? b.selected,
      position: override.position ?? b.position,
    }
  })

  // Include any user fields not present in base (custom fields added by user)
  const baseIds = new Set(base.map((b) => b.id))
  const extras = user.filter((u) => !baseIds.has(u.id))

  return [...merged, ...extras].sort((a, b) => (a.position ?? 0) - (b.position ?? 0))
}

export function useMappings() {
  const [mappings, setMappings] = useState<MappingItem[]>(cachedMappings ?? [])
  const [loading, setLoading] = useState(!cachedMappings)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async (force = false) => {
    if (cachedMappings && !force) {
      setMappings(cachedMappings)
      setLoading(false)
      return
    }
    if (!fetchPromise || force) {
      fetchPromise = fetchMappings()
    }
    setLoading(true)
    setError(null)
    try {
      const result = await fetchPromise
      cachedMappings = result
      setMappings(result)
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setLoading(false)
      fetchPromise = null
    }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  const refresh = useCallback(() => {
    cachedMappings = null
    fetchPromise = null
    load(true)
  }, [load])

  return { mappings, loading, error, refresh }
}

// Save user field overrides for a protocol
export async function saveUserMapping(
  hepid: number,
  profile: string,
  fields: FieldItem[],
): Promise<void> {
  await apiPut(`/me/mappings/${hepid}/${profile}`, fields)
  // invalidate cache so next load picks up new user_mapping
  cachedMappings = null
}

// Reset user field overrides for a protocol
export async function deleteUserMapping(hepid: number, profile: string): Promise<void> {
  await apiDelete(`/me/mappings/${hepid}/${profile}`)
  cachedMappings = null
}
