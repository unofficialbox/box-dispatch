// @vitest-environment jsdom
import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { DetailList } from './DetailsRail'

describe('DetailList', () => {
  it('exposes semantic value classes for readiness styling', () => {
    render(<DetailList rows={[["Status", "Validation complete"], ["Connections", "Ready"], ["Strategy", "Reuse existing"]]} />)

    expect(screen.getByText('Validation complete').classList.contains('detail-value-validation-complete')).toBe(true)
    expect(screen.getByText('Ready').classList.contains('detail-value-ready')).toBe(true)
    expect(screen.getByText('Reuse existing').classList.contains('detail-value-reuse-existing')).toBe(true)
  })
})
