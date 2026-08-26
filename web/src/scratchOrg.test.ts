import { describe, expect, it } from 'vitest'
import { scratchOrgRequest } from './scratchOrg'

describe('scratchOrgRequest', () => {
  it('uses the deployment name as the Salesforce scratch org name', () => {
    expect(scratchOrgRequest('northstar', 'Northstar CLM rollout')).toEqual({
      alias: 'northstar',
      orgName: 'Northstar CLM rollout',
      durationDays: 30,
      installManagedPackage: false,
    })
  })

  it('normalizes user input and preserves the legacy fallback', () => {
    expect(scratchOrgRequest('  northstar  ', '   ')).toEqual({
      alias: 'northstar',
      orgName: 'Box Dispatch',
      durationDays: 30,
      installManagedPackage: false,
    })
  })

  it('requests background managed-package preparation when selected', () => {
    expect(scratchOrgRequest('northstar', 'Northstar CLM rollout', true).installManagedPackage).toBe(true)
  })
})
