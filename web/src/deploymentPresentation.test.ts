import { describe, expect, it } from 'vitest'
import { deploymentOutcome } from './deploymentPresentation'

describe('deploymentOutcome', () => {
  it.each([
    { name: 'successful providers', providers: [{ name: 'box', status: 'present', remainingCount: 0, manualItemCount: 0 }], label: 'Complete', tone: 'success' },
    { name: 'legacy successful provider summary', providers: [{ name: 'box', status: 'present' }], label: 'Complete', tone: 'success' },
    { name: 'remaining work', providers: [{ name: 'box', status: 'present', remainingCount: 1, manualItemCount: 0 }], label: 'Needs attention', tone: 'error' },
    { name: 'manual work', providers: [{ name: 'box', status: 'present', remainingCount: 0, manualItemCount: 1 }], label: 'Needs attention', tone: 'error' },
    { name: 'failed provider', providers: [{ name: 'box', status: 'failed', remainingCount: 0, manualItemCount: 0 }], label: 'Needs attention', tone: 'error' },
    { name: 'mixed provider outcome', providers: [{ name: 'box', status: 'present', remainingCount: 0, manualItemCount: 0 }, { name: 'salesforce', status: 'present', remainingCount: 2, manualItemCount: 0 }], label: 'Needs attention', tone: 'error' },
    { name: 'record without providers', providers: [], label: 'Recorded', tone: 'info' },
  ])('classifies $name', ({ providers, label, tone }) => {
    expect(deploymentOutcome({ providers })).toEqual({ label, tone })
  })
})
