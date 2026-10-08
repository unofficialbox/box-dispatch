// @vitest-environment jsdom
import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { DetailList } from './DetailsRail'

describe('DetailList', () => {
  it('passes the requested facts to the Box Open Elements fact list', () => {
    const rows: Array<[string, string]> = [["Status", "Validation complete"], ["Connections", "Ready"], ["Strategy", "Reuse existing"]]
    const { container } = render(<DetailList rows={rows} />)

    const factList = container.querySelector('box-fact-list') as HTMLElement & { rows: Array<{ label: string; value: string }> }
    expect(factList).toBeTruthy()
    expect(factList.rows).toEqual(rows.map(([label, value]) => ({ label, value })))
  })
})
