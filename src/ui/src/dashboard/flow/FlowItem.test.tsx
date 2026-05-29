import { describe, expect, it } from 'vitest'
import { render } from '@testing-library/react'
import { FlowItem } from './FlowItem'
import type { FlowItemData } from './flow-data'

function makeItem(direction: boolean): FlowItemData {
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
