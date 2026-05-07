import type { FlowItemData } from './flow-data'

interface FlowItemProps {
  item: FlowItemData
  isSimplify: boolean
  isAbsoluteTime: boolean
  singleHost?: boolean
  onClickItem?: (item: FlowItemData) => void
}

export function FlowItem({
  item,
  isSimplify,
  isAbsoluteTime,
  singleHost,
  onClickItem,
}: FlowItemProps) {
  const flexStart = item.start || 0.0000001
  const flexMiddle = item.middle || 0.0000001
  const flexRight = item.isRadial
    ? Math.max(0.0000001, item.rightEnd - (item.isLastHost ? 0 : 1))
    : item.rightEnd || 0.0000001

  const arrowColor = item.callidColors.arrowColor
  const radialDottedClass = item.arrowStyleSolid ? '' : ' dotted'
  const containerClass = `item-flow-packet-container${singleHost ? ' single-host' : ''}`

  return (
    <div className={containerClass}>
      <div className="bg-color-polygon" style={{ backgroundColor: item.callidColors.color }} />
      <div className="bg-color-tab" style={{ backgroundColor: item.callidColors.tabColor }} />
      <div
        className="item-flow-packet"
        onClick={() => onClickItem?.(item)}
      >
        <div className="item-flow-track">
          <div style={{ flex: flexStart }} />
          <div
            className="item-flow-segment"
            style={{
              flex: flexMiddle,
              textAlign: item.direction ? 'right' : 'left',
            }}
          >
            <div
              className={`call_text${item.direction ? ' right' : ''}`}
              style={{ color: item.methodColor }}
            >
              <span>{item.method || '—'}</span>
              {item.protoLabel ? <span className="proto-label">{item.protoLabel}</span> : null}
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
              <div className="radial-port-labels">
                <span className="radial-port-src">{item.srcPort || ''}</span>
                <span className="radial-port-dst">{item.dstPort || ''}</span>
              </div>
            ) : (
              <>
                <div className={item.direction ? 'port-label-right' : 'port-label-left'}>
                  {item.direction ? item.dstPort || '' : item.srcPort || ''}
                </div>
                <div className={item.direction ? 'port-label-left' : 'port-label-right'}>
                  {item.direction ? item.srcPort || '' : item.dstPort || ''}
                </div>
              </>
            )}

            {!isSimplify && (
              <div className="call-text-date">{isAbsoluteTime ? item.fullDateStr : item.diffStr}</div>
            )}
            {isSimplify && <div className="call-text-date">{item.diffStr}</div>}
          </div>
          <div style={{ flex: flexRight }} />
        </div>
      </div>
    </div>
  )
}
