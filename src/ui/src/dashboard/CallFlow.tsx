import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import './CallFlow.css'
import { CallIdSelector } from './flow/CallIdSelector'
import { FilterPanel } from './flow/FilterPanel'
import { FlowHostsLine } from './flow/FlowHostsLine'
import { FlowItem } from './flow/FlowItem'
import type { FlowItemData, RawMessage } from './flow/flow-data'
import { buildFlow, buildCallIdLegend, consolidateFlowItems } from './flow/flow-data'
import { useFlowFilters } from './flow/useFlowFilters'
import { useLocale } from '@/components/locale/locale-provider'

interface CallFlowProps {
  items: RawMessage[] | null | undefined
  timeZone?: string
  onClickMessage?: (item: FlowItemData) => void
}

export default function CallFlow({ items, timeZone, onClickMessage }: CallFlowProps) {
  const { resolved: locale } = useLocale()
  const {
    filters,
    setFilters,
    filterIP,
    filterMethod,
    filterPayloadType,
    filterCallId,
    filteredItems,
  } = useFlowFilters(items)

  const [expandedItemKey, setExpandedItemKey] = useState<string | null>(null)

  const { hosts, flowItems } = useMemo(
    () =>
      buildFlow(filteredItems, {
        timeZone,
        grouping: filters.hostGrouping,
        locale,
      }),
    [filteredItems, timeZone, filters.hostGrouping, locale],
  )

  const canConsolidateCaptureIds = useMemo(
    () => flowItems.some((item) => (item.runtimeFingerprint ?? '') !== ''),
    [flowItems],
  )

  const consolidatedFlowItems = useMemo(
    () =>
      consolidateFlowItems(flowItems, {
        enabled: filters.isConsolidateCaptureIds && canConsolidateCaptureIds,
        timeThresholdMs: filters.consolidationTimeThresholdMs,
      }),
    [flowItems, filters.isConsolidateCaptureIds, filters.consolidationTimeThresholdMs, canConsolidateCaptureIds],
  )

  useEffect(() => {
    if (!expandedItemKey) return
    const exists = consolidatedFlowItems.some((item) => String(item.id ?? item.idx) === expandedItemKey)
    if (!exists) setExpandedItemKey(null)
  }, [expandedItemKey, consolidatedFlowItems])

  const callIds = useMemo(() => buildCallIdLegend(items), [items])

  const innerWidth = Math.max(hosts.length * 220, 560)

  const innerRef = useRef<HTMLDivElement>(null)
  const [gridLines, setGridLines] = useState<number[]>([])

  useLayoutEffect(() => {
    const el = innerRef.current
    if (!el || hosts.length === 0) return

    const measure = () => {
      const innerRect = el.getBoundingClientRect()
      if (innerRect.width === 0) return
      const wrappers = el.querySelectorAll<HTMLElement>('.item-wrapper')
      if (wrappers.length === 0) return

      const positions: number[] = []
      wrappers.forEach((wrapper) => {
        const rect = wrapper.getBoundingClientRect()
        positions.push(rect.left + rect.width / 2 - innerRect.left)
      })
      setGridLines(positions)

      if (positions.length >= 2) {
        el.style.setProperty('--flow-left-px', `${positions[0]}px`)
        el.style.setProperty('--flow-right-px', `${innerRect.width - positions[positions.length - 1]}px`)
      } else if (positions.length === 1) {
        el.style.setProperty('--flow-left-px', `${positions[0]}px`)
        el.style.setProperty('--flow-right-px', `${innerRect.width - positions[0] - 280}px`)
      } else {
        el.style.removeProperty('--flow-left-px')
        el.style.removeProperty('--flow-right-px')
      }
    }

    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(el)
    return () => observer.disconnect()
  }, [hosts])

  if (!items || items.length === 0) {
    return <div className="callflow-empty">No messages to display</div>
  }

  return (
    <div className="callflow-root">
      <div className="callflow-toolbar">
        <FilterPanel
          filters={filters}
          setFilters={setFilters}
          filterIP={filterIP}
          filterMethod={filterMethod}
          filterPayloadType={filterPayloadType}
          filterCallId={filterCallId}
          canConsolidateCaptureIds={canConsolidateCaptureIds}
        />
        <span>{consolidatedFlowItems.length} messages</span>
        <span className="callflow-toolbar-spacer" />
        <span>{hosts.length} hosts</span>
      </div>

      <div className="callflow-body">
        <div className="callflow-scroll">
          <div
            ref={innerRef}
            className="callflow-inner"
            style={{ width: `max(100%, ${innerWidth}px)` }}
          >
            <div className="flow-grid-overlay">
              {gridLines.map((px, i) => (
                <div
                  key={hosts[i]?.key || i}
                  className="grid-line"
                  style={{ left: `${px}px` }}
                />
              ))}
            </div>

            <FlowHostsLine hosts={hosts} />

            <div className="callflow-items">
              {consolidatedFlowItems.length === 0 ? (
                <div className="callflow-empty-state">No rows match the current filters</div>
              ) : (
                consolidatedFlowItems.map((item) => (
                  <FlowItem
                    key={item.id}
                    item={item}
                    isSimplify={filters.isSimplify}
                    isAbsoluteTime={filters.isAbsoluteTime}
                    singleHost={hosts.length === 1}
                    onClickItem={onClickMessage}
                    expandedKey={expandedItemKey}
                    setExpandedKey={setExpandedItemKey}
                  />
                ))
              )}
            </div>
          </div>
        </div>

        <CallIdSelector callIds={callIds} filters={filters} setFilters={setFilters} />
      </div>
    </div>
  )
}
