import { useState, useEffect, useMemo } from 'react'
import { Loader2, RefreshCw } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import DragDropFieldList from './DragDropFieldList'
import {
  useMappings,
  getMergedFields,
  saveUserMapping,
  deleteUserMapping,
  type FieldItem,
  type MappingItem,
} from '@/hooks/useMappings'

export interface SearchWidgetConfig {
  protocol_id?: { name: string; value: number }
  protocol_profile?: string
  fields?: FieldItem[]
  targetWidget?: string
  searchbutton?: boolean
  title?: string
}

interface SearchWidgetSettingsProps {
  open: boolean
  onClose: () => void
  config: SearchWidgetConfig
  onSave: (config: SearchWidgetConfig) => void
}

const DEFAULT_FIELDS: FieldItem[] = [
  { id: 'limit', name: 'Query Limit', type: 'integer', form_type: 'input', form_default: '100' },
  {
    id: 'results_container',
    name: 'Results Container',
    type: 'string',
    form_type: 'select',
    form_default: 'default',
  },
]

export default function SearchWidgetSettings({
  open,
  onClose,
  config,
  onSave,
}: SearchWidgetSettingsProps) {
  const { mappings, loading, error, refresh } = useMappings()

  const [title, setTitle] = useState(config.title ?? '')
  const [selectedProtoKey, setSelectedProtoKey] = useState<string>('')
  const [currentFields, setCurrentFields] = useState<FieldItem[]>([])
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  // Build the sorted options list (hep_alias - profile). Alphabetical
  // (case-insensitive, locale-aware) so users can scan the picker
  // predictably as the list grows — auto-published OTLP and dynamic
  // Line Protocol mappings (LP_*) can otherwise push the most-used
  // SIP entries far down depending on insert order.
  const protoOptions = useMemo(
    () =>
      mappings
        .map((m) => ({
          key: `${m.hepid}_${m.profile}`,
          label: `${m.hep_alias} - ${m.profile}`,
          mapping: m,
        }))
        .sort((a, b) =>
          a.label.localeCompare(b.label, undefined, { sensitivity: 'base', numeric: true }),
        ),
    [mappings],
  )

  // Derive the currently selected mapping from the key
  const selectedMapping = useMemo<MappingItem | undefined>(
    () => protoOptions.find((o) => o.key === selectedProtoKey)?.mapping,
    [protoOptions, selectedProtoKey],
  )

  // When dialog opens (or mappings load), initialise selection from existing config
  useEffect(() => {
    if (!open || mappings.length === 0) return
    setTitle(config.title ?? '')

    const initialKey =
      config.protocol_id && config.protocol_profile
        ? `${config.protocol_id.value}_${config.protocol_profile}`
        : protoOptions[0]?.key ?? ''

    setSelectedProtoKey(initialKey)
  }, [open, mappings, config, protoOptions])

  // When selected protocol changes, rebuild the field list. Fields
  // are sorted alphabetically (by display name, falling back to id)
  // before handing them to DragDropFieldList — this fixes the
  // "Inactive column is in some random insertion order" complaint
  // raised against widgets pointing at protocols with hundreds of
  // fields (LP_* tables, OTLP traces).  Active order is *not*
  // affected: DragDropFieldList rebuilds the Active column from
  // `activeFieldIds`, which preserves the drag-and-drop order the
  // user previously stored in the widget config.
  useEffect(() => {
    if (!selectedMapping) return
    const merged = getMergedFields(selectedMapping)
    const withDefaults = merged.find((f) => f.id === 'limit')
      ? merged
      : [...merged, ...DEFAULT_FIELDS]

    const activeIds = (config.fields ?? []).map((f) => f.id)
    // Preserve active selection from saved config if protocol matches
    const configProtoKey =
      config.protocol_id && config.protocol_profile
        ? `${config.protocol_id.value}_${config.protocol_profile}`
        : null
    const useExistingActive = configProtoKey === selectedProtoKey && activeIds.length > 0

    const sorted = [...withDefaults]
      .filter((f) => !f.skip)
      .sort((a, b) =>
        (a.name || a.id).localeCompare(b.name || b.id, undefined, {
          sensitivity: 'base',
          numeric: true,
        }),
      )

    setCurrentFields(
      sorted.map((f) => ({
        ...f,
        selected: useExistingActive ? activeIds.includes(f.id) : f.selected ?? false,
      })),
    )
  }, [selectedMapping, selectedProtoKey, config.fields, config.protocol_id, config.protocol_profile])

  const activeIds = useMemo(
    () => currentFields.filter((f) => f.selected).map((f) => f.id),
    [currentFields],
  )

  const handleFieldsChange = (updated: FieldItem[]) => {
    setCurrentFields(updated)
  }

  const handleProtoChange = (key: string) => {
    setSelectedProtoKey(key)
  }

  const handleSave = async () => {
    if (!selectedMapping) return
    setSaving(true)
    setSaveError(null)
    try {
      const activeFields = currentFields.filter((f) => f.selected)
      // Persist user mapping preferences server-side
      await saveUserMapping(selectedMapping.hepid, selectedMapping.profile, activeFields)

      const newConfig: SearchWidgetConfig = {
        ...config,
        title,
        protocol_id: {
          name: selectedMapping.hep_alias,
          value: selectedMapping.hepid,
        },
        protocol_profile: selectedMapping.profile,
        fields: activeFields.map((f) => ({
          ...f,
          proto: `${selectedMapping.hep_alias}-${selectedMapping.profile}`,
          field_name: f.id,
        })),
      }
      onSave(newConfig)
      onClose()
    } catch (err: unknown) {
      setSaveError(err instanceof Error ? err.message : 'Failed to save')
    } finally {
      setSaving(false)
    }
  }

  const handleReset = async () => {
    if (!selectedMapping) return
    setSaving(true)
    setSaveError(null)
    try {
      await deleteUserMapping(selectedMapping.hepid, selectedMapping.profile)
      refresh()
    } catch (err: unknown) {
      setSaveError(err instanceof Error ? err.message : 'Failed to reset')
    } finally {
      setSaving(false)
    }
  }

  const isValid = activeIds.length > 0

  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent className="flex max-h-[90vh] w-full max-w-[min(100vw-2rem,56rem)] flex-col gap-4 !sm:max-w-4xl">
        <DialogHeader>
          <DialogTitle>Search Widget Settings</DialogTitle>
        </DialogHeader>

        {loading ? (
          <div className="flex items-center justify-center py-8">
            <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
          </div>
        ) : error ? (
          <div className="space-y-2">
            <p className="text-sm text-destructive">{error}</p>
            <Button variant="outline" size="sm" onClick={refresh}>
              <RefreshCw className="mr-1.5 h-3.5 w-3.5" /> Retry
            </Button>
          </div>
        ) : (
          <>
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <Label htmlFor="widget-title">Title</Label>
                <Input
                  id="widget-title"
                  value={title}
                  onChange={(e) => setTitle(e.target.value)}
                  placeholder="Widget title"
                />
              </div>
              <div className="space-y-1.5">
                <Label>Protocol</Label>
                <Select value={selectedProtoKey} onValueChange={handleProtoChange}>
                  <SelectTrigger>
                    <SelectValue placeholder="Select protocol…" />
                  </SelectTrigger>
                  <SelectContent>
                    {protoOptions.map((opt) => (
                      <SelectItem key={opt.key} value={opt.key}>
                        {opt.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>

            {selectedMapping && (
              <div className="min-h-0 flex-1 overflow-y-auto">
                <DragDropFieldList
                  fields={currentFields}
                  activeFieldIds={activeIds}
                  onChange={handleFieldsChange}
                />
              </div>
            )}

            {saveError && <p className="text-sm text-destructive">{saveError}</p>}
          </>
        )}

        <DialogFooter className="flex items-center justify-between">
          <Button
            variant="outline"
            size="sm"
            onClick={handleReset}
            disabled={saving || !selectedMapping}
            title="Reset to default fields for this protocol"
          >
            <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
            Reset defaults
          </Button>
          <div className="flex gap-2">
            <Button variant="outline" onClick={onClose} disabled={saving}>
              Cancel
            </Button>
            <Button onClick={handleSave} disabled={saving || !isValid}>
              {saving && <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />}
              Save
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
