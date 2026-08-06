import { describe, expect, it, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/react'
import { FlowItem } from './FlowItem'
import type { FlowItemData } from './flow-data'

function makeItem(direction: boolean, overrides: Partial<FlowItemData> = {}): FlowItemData {
  return {
    id: '1',
    idx: 1,
    method: 'INVITE',
    description: 'desc',
    srcIp: '10.0.0.1',
    dstIp: '10.0.0.2',
    srcPort: 5060,
    dstPort: 5070,
    callid: 'c1',
    callidColors: {
      color: '#000000',
      tabColor: '#000000',
      arrowColor: '#00ff00',
    },
    methodColor: '#00ff00',
    timeStr: '',
    fullDateStr: '',
    diffStr: '+0.0ms',
    protoLabel: 'UDP',
    payloadType: 'SIP',
    start: 0,
    middle: 1,
    rightEnd: 0,
    direction,
    isRadial: false,
    isLastHost: false,
    arrowStyleSolid: true,
    raw: {},
    ...overrides,
  }
}

describe('FlowItem port labels', () => {
  it('shows src on left and dst on right for left-to-right packets', () => {
    const { container } = render(
      <FlowItem item={makeItem(false)} isSimplify={false} isAbsoluteTime={false} />,
    )

    expect(container.querySelector('.port-label-left')?.textContent).toBe('5060')
    expect(container.querySelector('.port-label-right')?.textContent).toBe('5070')
  })

  it('shows dst on left and src on right for right-to-left packets', () => {
    const { container } = render(
      <FlowItem item={makeItem(true)} isSimplify={false} isAbsoluteTime={false} />,
    )

    expect(container.querySelector('.port-label-left')?.textContent).toBe('5070')
    expect(container.querySelector('.port-label-right')?.textContent).toBe('5060')
  })
})

describe('FlowItem consolidation', () => {
  const subItem = makeItem(false, { id: 'sub1', method: 'BYE' })
  const parentItem = makeItem(false, { id: 'parent', subItems: [subItem] })

  it('renders toggle button and +N badge when item has subItems', () => {
    const { container } = render(
      <FlowItem item={parentItem} isSimplify={false} isAbsoluteTime={false}
        expandedKey={null} setExpandedKey={() => {}} />,
    )
    expect(container.querySelector('.subitems-toggle')).toBeTruthy()
    expect(container.querySelector('.subitems-count')?.textContent).toBe('+1')
  })

  it('does not render toggle when item has no subItems', () => {
    const { container } = render(
      <FlowItem item={makeItem(false)} isSimplify={false} isAbsoluteTime={false} />,
    )
    expect(container.querySelector('.subitems-toggle')).toBeNull()
    expect(container.querySelector('.subitems-count')).toBeNull()
  })

  it('shows sub-items when expandedKey matches item id', () => {
    const { container } = render(
      <FlowItem item={parentItem} isSimplify={false} isAbsoluteTime={false}
        expandedKey="parent" setExpandedKey={() => {}} />,
    )
    const packets = container.querySelectorAll('.item-flow-packet-container')
    expect(packets.length).toBe(2)
  })

  it('hides sub-items when expandedKey does not match', () => {
    const { container } = render(
      <FlowItem item={parentItem} isSimplify={false} isAbsoluteTime={false}
        expandedKey={null} setExpandedKey={() => {}} />,
    )
    expect(container.querySelectorAll('.item-flow-packet-container').length).toBe(1)
  })

  it('toggle click calls setExpandedKey and does not call onClickItem', () => {
    const setExpandedKey = vi.fn()
    const onClickItem = vi.fn()
    const { container } = render(
      <FlowItem item={parentItem} isSimplify={false} isAbsoluteTime={false}
        expandedKey={null} setExpandedKey={setExpandedKey} onClickItem={onClickItem} />,
    )
    fireEvent.click(container.querySelector('.subitems-toggle')!)
    expect(setExpandedKey).toHaveBeenCalledWith('parent')
    expect(onClickItem).not.toHaveBeenCalled()
  })
})
