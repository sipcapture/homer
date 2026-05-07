import { ChevronDown, ChevronUp } from 'lucide-react'
import type { Dispatch, SetStateAction } from 'react'
import { useRef, useState } from 'react'
import type { CallIdLegend } from './flow-data'
import type { FlowFilters } from './useFlowFilters'
import { toggleInSet } from './useFlowFilters'

interface CallIdSelectorProps {
  callIds: CallIdLegend[]
  filters: FlowFilters
  setFilters: Dispatch<SetStateAction<FlowFilters>>
}

export function CallIdSelector({ callIds, filters, setFilters }: CallIdSelectorProps) {
  const [open, setOpen] = useState(true)
  const [copied, setCopied] = useState<string | null>(null)
  const copyTimer = useRef<number | null>(null)

  if (callIds.length === 0) return null

  const isSelected = (cid: string): boolean => !filters.callIdExcluded.has(cid)

  const handleClick = (cid: string) => {
    if (navigator.clipboard && cid) {
      void navigator.clipboard.writeText(cid)
      setCopied(cid)
      if (copyTimer.current) window.clearTimeout(copyTimer.current)
      copyTimer.current = window.setTimeout(() => setCopied(null), 2000)
    }
    setFilters((prev) => ({ ...prev, callIdExcluded: toggleInSet(prev.callIdExcluded, cid) }))
  }

  return (
    <div className={`label-callid-container${open ? '' : ' hidden'}`}>
      <button
        type="button"
        className="label-callid-pull"
        onClick={() => setOpen(!open)}
        aria-label={open ? 'Collapse call IDs' : 'Expand call IDs'}
      >
        {open ? <ChevronDown size={14} /> : <ChevronUp size={14} />}
      </button>
      <div className="label-callid-list">
        {callIds.map((cid) => {
          const selected = isSelected(cid.callid)
          const isCopied = copied === cid.callid
          return (
            <button
              key={cid.callid}
              type="button"
              className={`label-callid-wrapper${selected ? ' is-selected' : ' is-muted'}${isCopied ? ' is-copied' : ''}`}
              style={{
                borderColor: selected ? cid.colors.tabColor : undefined,
                color: selected ? cid.colors.tabColor : undefined,
              }}
              onClick={() => handleClick(cid.callid)}
              title={isCopied ? 'Copied!' : cid.callid}
            >
              {cid.callid}
            </button>
          )
        })}
      </div>
    </div>
  )
}
