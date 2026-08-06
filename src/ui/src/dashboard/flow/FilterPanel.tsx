import { ChevronDown, Filter as FilterIcon, X } from 'lucide-react'
import { Accordion as AccordionPrimitive, RadioGroup as RadioGroupPrimitive } from 'radix-ui'
import type { Dispatch, SetStateAction } from 'react'
import { useMemo, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import type { HostGrouping } from './flow-data'
import type { FilterToken, FlowFilters } from './useFlowFilters'
import { tokensToExcluded } from './useFlowFilters'

type FilterGroupKey = 'ipExcluded' | 'methodExcluded' | 'payloadTypeExcluded' | 'callIdExcluded'

interface FilterPanelProps {
  filters: FlowFilters
  setFilters: Dispatch<SetStateAction<FlowFilters>>
  filterIP: FilterToken[]
  filterMethod: FilterToken[]
  filterPayloadType: FilterToken[]
  filterCallId: FilterToken[]
  canConsolidateCaptureIds?: boolean
}

function toggleTokens(tokens: FilterToken[], value: string): FilterToken[] {
  return tokens.map((t) => (t.value === value ? { ...t, selected: !t.selected } : t))
}

function setAllTokens(tokens: FilterToken[], selected: boolean): FilterToken[] {
  return tokens.map((t) => ({ ...t, selected }))
}

export function FilterPanel({
  filters,
  setFilters,
  filterIP,
  filterMethod,
  filterPayloadType,
  filterCallId,
  canConsolidateCaptureIds = false,
}: FilterPanelProps) {
  const [open, setOpen] = useState(false)

  const updateTokens = (group: FilterGroupKey, nextTokens: FilterToken[]) => {
    setFilters((prev) => ({ ...prev, [group]: tokensToExcluded(nextTokens) }))
  }

  return (
    <>
      <button
        type="button"
        className="callflow-filter-toggle"
        aria-label="Toggle filters"
        onClick={() => setOpen(!open)}
      >
        <FilterIcon size={14} />
      </button>

      {open && (
        <aside className="callflow-filter-panel">
          <div className="callflow-filter-header">
            <span>Filters</span>
            <button
              type="button"
              onClick={() => setOpen(false)}
              aria-label="Close filters"
              className="callflow-filter-close"
            >
              <X size={14} />
            </button>
          </div>

          <div className="callflow-filter-body">
            <section className="callflow-filter-section">
              <div className="callflow-filter-row">
                <Label htmlFor="flow-compact">Compact</Label>
                <Switch
                  id="flow-compact"
                  checked={filters.isSimplify}
                  onCheckedChange={(v) => setFilters((p) => ({ ...p, isSimplify: !!v }))}
                />
              </div>
              <div className="callflow-filter-row">
                <Label htmlFor="flow-abs-time">Absolute time</Label>
                <Switch
                  id="flow-abs-time"
                  checked={filters.isAbsoluteTime}
                  onCheckedChange={(v) => setFilters((p) => ({ ...p, isAbsoluteTime: !!v }))}
                />
              </div>
              <div className="callflow-filter-row">
                <Label htmlFor="flow-hc">High-contrast Call-IDs</Label>
                <Switch
                  id="flow-hc"
                  checked={filters.isHighContrast}
                  onCheckedChange={(v) => setFilters((p) => ({ ...p, isHighContrast: !!v }))}
                />
              </div>
              {canConsolidateCaptureIds ? (
                <>
                  <div className="callflow-filter-row">
                    <Label htmlFor="flow-consolidate-capture-ids">Consolidate by fingerprint</Label>
                    <Switch
                      id="flow-consolidate-capture-ids"
                      checked={filters.isConsolidateCaptureIds}
                      onCheckedChange={(v) =>
                        setFilters((p) => ({ ...p, isConsolidateCaptureIds: !!v }))
                      }
                    />
                  </div>
                  <div className="callflow-filter-row flow-threshold-row">
                    <Label htmlFor="flow-consolidate-threshold">Threshold (ms)</Label>
                    <input
                      id="flow-consolidate-threshold"
                      type="number"
                      min={0}
                      step={1}
                      className="callflow-threshold-input"
                      disabled={!filters.isConsolidateCaptureIds}
                      value={filters.consolidationTimeThresholdMs}
                      onChange={(e) => {
                        const next = Number.parseInt(e.target.value || '0', 10)
                        setFilters((p) => ({
                          ...p,
                          consolidationTimeThresholdMs: Number.isFinite(next) ? Math.max(0, next) : 0,
                        }))
                      }}
                    />
                  </div>
                </>
              ) : null}
            </section>

            <section className="callflow-filter-section">
              <div className="callflow-filter-section-title">Host grouping</div>
              <RadioGroupPrimitive.Root
                className="callflow-radio-group"
                value={filters.hostGrouping}
                onValueChange={(v) =>
                  setFilters((p) => ({ ...p, hostGrouping: v as HostGrouping }))
                }
              >
                <RadioRow value="ungrouped" label="Ungrouped (IP + port)" />
                <RadioRow value="group-by-ip" label="Group by IP" />
                <RadioRow value="group-by-alias" label="Group by alias" />
              </RadioGroupPrimitive.Root>
            </section>

            <AccordionPrimitive.Root type="multiple" className="callflow-accordion">
              <FilterAccordion
                title="IPs"
                tokens={filterIP}
                onChange={(next) => updateTokens('ipExcluded', next)}
              />
              <FilterAccordion
                title="Methods"
                tokens={filterMethod}
                onChange={(next) => updateTokens('methodExcluded', next)}
              />
              <FilterAccordion
                title="Payload type"
                tokens={filterPayloadType}
                onChange={(next) => updateTokens('payloadTypeExcluded', next)}
              />
              <FilterAccordion
                title="Call IDs"
                tokens={filterCallId}
                onChange={(next) => updateTokens('callIdExcluded', next)}
              />
            </AccordionPrimitive.Root>
          </div>
        </aside>
      )}
    </>
  )
}

function RadioRow({
  value,
  label,
  disabled,
}: {
  value: string
  label: string
  disabled?: boolean
}) {
  return (
    <label className={`callflow-radio-row${disabled ? ' disabled' : ''}`}>
      <RadioGroupPrimitive.Item value={value} disabled={disabled} className="callflow-radio">
        <RadioGroupPrimitive.Indicator className="callflow-radio-dot" />
      </RadioGroupPrimitive.Item>
      <span>{label}</span>
    </label>
  )
}

interface FilterAccordionProps {
  title: string
  tokens: FilterToken[]
  onChange: (next: FilterToken[]) => void
}

function FilterAccordion({ title, tokens, onChange }: FilterAccordionProps) {
  const id = useMemo(() => title.toLowerCase().replace(/\s+/g, '-'), [title])
  const selectedCount = tokens.filter((t) => t.selected).length

  return (
    <AccordionPrimitive.Item value={id} className="callflow-accordion-item">
      <AccordionPrimitive.Header>
        <AccordionPrimitive.Trigger className="callflow-accordion-trigger">
          <span>{title}</span>
          <span className="callflow-accordion-count">
            {selectedCount}/{tokens.length}
          </span>
          <ChevronDown size={14} className="callflow-accordion-chevron" />
        </AccordionPrimitive.Trigger>
      </AccordionPrimitive.Header>
      <AccordionPrimitive.Content className="callflow-accordion-content">
        <div className="callflow-accordion-actions">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => onChange(setAllTokens(tokens, true))}
          >
            Select all
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => onChange(setAllTokens(tokens, false))}
          >
            Clear
          </Button>
        </div>
        <ul className="callflow-token-list">
          {tokens.map((t) => (
            <li key={t.value}>
              <label className="callflow-token-row">
                <Checkbox
                  checked={t.selected}
                  onCheckedChange={() => onChange(toggleTokens(tokens, t.value))}
                />
                <span className="callflow-token-value">{t.value || '(empty)'}</span>
              </label>
            </li>
          ))}
          {tokens.length === 0 && <li className="callflow-token-empty">No values</li>}
        </ul>
      </AccordionPrimitive.Content>
    </AccordionPrimitive.Item>
  )
}
