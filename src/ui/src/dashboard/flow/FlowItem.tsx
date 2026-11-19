import type { FlowItemData } from './flow-data'
import { flowSegmentPx } from './flow-data'

interface FlowItemProps {
  item: FlowItemData
  isSimplify: boolean
  isAbsoluteTime: boolean
  /** Measured host centres (px from the left edge of the flow), one per host. */
  hostCenters?: readonly number[]
  onClickItem?: (item: FlowItemData) => void
  expandedKey?: string | null
  setExpandedKey?: (key: string | null) => void
  isSubItem?: boolean
}

export function FlowItem({
  item,
  isSimplify,
  isAbsoluteTime,
  hostCenters,
  onClickItem,
  expandedKey,
  setExpandedKey,
  isSubItem,
}: FlowItemProps) {
  const itemKey = String(item.id ?? item.idx)
  const hasSubItems = !!item.subItems && item.subItems.length > 0
  const canToggleSubItems = hasSubItems && typeof setExpandedKey === 'function'
  const showSubItems = canToggleSubItems && expandedKey === itemKey

  // Until host centres are measured the segment just fills the row; the
  // layout effect in CallFlow measures before the first paint.
  const segment = hostCenters ? flowSegmentPx(item, hostCenters) : null

  const arrowColor = item.callidColors.arrowColor
  const radialDottedClass = item.arrowStyleSolid ? '' : ' dotted'
  const containerClass = `item-flow-packet-container${hasSubItems ? ' consolidated-parent' : ''}${
    isSubItem ? ' consolidated-subitem' : ''
  }`

  return (
    <>
      <div className={containerClass}>
        <div className="bg-color-polygon" style={{ backgroundColor: item.callidColors.color }} />
        <div className="bg-color-tab" style={{ backgroundColor: item.callidColors.tabColor }} />
        <div
          className="item-flow-packet"
          onClick={() => onClickItem?.(item)}
        >
          <div className="item-flow-track">
            <div style={{ flex: `0 0 ${segment?.left ?? 0}px` }} />
            <div
              className="item-flow-segment"
              style={{
                flex: segment ? `0 0 ${segment.width}px` : '1 1 0',
                textAlign: item.direction ? 'right' : 'left',
              }}
            >
              <div
                className={`call_text${item.direction ? ' right' : ''}`}
                style={{ color: item.methodColor }}
              >
                {canToggleSubItems ? (
                  <button
                    type="button"
                    className="subitems-toggle"
                    aria-expanded={showSubItems}
                    aria-label={showSubItems ? 'Collapse consolidated messages' : 'Expand consolidated messages'}
                    title={showSubItems ? 'Collapse consolidated messages' : 'Expand consolidated messages'}
                    onClick={(event) => {
                      event.stopPropagation()
                      setExpandedKey(showSubItems ? null : itemKey)
                    }}
                  >
                    {showSubItems ? '▽' : '▷'}
                  </button>
                ) : null}
                <span>{item.method || '—'}</span>
                {item.protoLabel ? <span className="proto-label">{item.protoLabel}</span> : null}
                {hasSubItems ? <span className="subitems-count">+{item.subItems!.length}</span> : null}
              </div>

              {!isSimplify && <div className="call_text-mini">{item.description}</div>}

              {!item.isRadial ? (
                <div
                  className={`arrow ${item.direction ? 'left ' : ''}${
                    item.arrowStyleSolid ? 'arrow-solid' : 'arrow-dotted'
                  }`}
                  style={{ color: arrowColor }}
                />
              ) : (
                <div
                  className={`${item.isLastHost ? 'radial-arrow-end' : 'radial-arrow'}${radialDottedClass}`}
                  style={{ color: arrowColor }}
                />
              )}

              {item.isRadial ? (
                <div className={`radial-port-labels${item.isLastHost ? ' end' : ''}`}>
                  <span className="radial-port-src">{item.srcPort || ''}</span>
                  <span className="radial-port-dst">{item.dstPort || ''}</span>
                </div>
              ) : (
                <>
                  <div className={item.direction ? 'port-label-right' : 'port-label-left'}>
                    {item.srcPort || ''}
                  </div>
                  <div className={item.direction ? 'port-label-left' : 'port-label-right'}>
                    {item.dstPort || ''}
                  </div>
                </>
              )}

              {!isSimplify && (
                <div className="call-text-date">{isAbsoluteTime ? item.fullDateStr : item.diffStr}</div>
              )}
              {isSimplify && <div className="call-text-date">{item.diffStr}</div>}
            </div>
          </div>
        </div>
      </div>
      {hasSubItems && showSubItems
        ? item.subItems!.map((subItem, idx) => (
            <FlowItem
              key={`${itemKey}:sub:${subItem.id ?? idx}`}
              item={subItem}
              isSimplify={isSimplify}
              isAbsoluteTime={isAbsoluteTime}
              hostCenters={hostCenters}
              onClickItem={onClickItem}
              expandedKey={expandedKey}
              setExpandedKey={setExpandedKey}
              isSubItem
            />
          ))
        : null}
    </>
  )
}
