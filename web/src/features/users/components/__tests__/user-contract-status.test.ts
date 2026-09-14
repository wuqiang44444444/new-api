import { describe, expect, it } from 'vitest'

import { userSchema } from '../../types'
import { getUserContractStatus } from '../user-contract-status'

const t = (key: string, options?: Record<string, unknown>) => {
  if (!options) return key
  return key.replaceAll(/{{(\w+)}}/g, (_, name: string) =>
    String(options[name])
  )
}

function user(summary?: { total: number; enabled: number }) {
  return userSchema.parse({
    id: 1,
    username: 'customer',
    display_name: 'Customer',
    quota: 0,
    used_quota: 0,
    request_count: 0,
    group: 'default',
    status: 1,
    role: 1,
    contract_mode: true,
    contract_version: 8,
    contract_rule_count: 0,
    contract_summary: summary,
  })
}

describe('user contract status', () => {
  it('shows migrated entity counts despite stale legacy flags and zero rule count', () => {
    expect(getUserContractStatus(user({ total: 1, enabled: 1 }), t)).toEqual({
      label: 'Contracts: 1 · enabled: 1',
      variant: 'info',
    })
  })

  it('shows mixed enabled and disabled contracts without claiming user-wide model access', () => {
    expect(getUserContractStatus(user({ total: 3, enabled: 1 }), t).label).toBe(
      'Contracts: 3 · enabled: 1'
    )
  })

  it('shows disabled entities despite the legacy enabled flag', () => {
    expect(getUserContractStatus(user({ total: 2, enabled: 0 }), t)).toEqual({
      label: 'Contracts: 2 · enabled: 0',
      variant: 'neutral',
    })
  })

  it('shows no contracts when only legacy state exists', () => {
    expect(getUserContractStatus(user({ total: 0, enabled: 0 }), t)).toEqual({
      label: 'No contracts',
      variant: 'neutral',
    })
  })

  it('does not interpret a missing summary as zero access or native mode', () => {
    expect(getUserContractStatus(user(), t)).toEqual({
      label: 'Contract summary unavailable',
      variant: 'neutral',
    })
  })
})
