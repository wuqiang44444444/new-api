import { describe, expect, it } from 'vitest'

import type { User } from '../../types'
import { getUserContractStatus } from '../user-contract-status'

const t = (key: string, options?: Record<string, unknown>) =>
  options?.count === undefined ? key : `${key}:${options.count}`

function user(patch: Partial<User>): User {
  return {
    id: 1,
    username: 'customer',
    display_name: 'Customer',
    quota: 0,
    used_quota: 0,
    request_count: 0,
    group: 'default',
    status: 1,
    role: 1,
    contract_mode: false,
    contract_version: 0,
    contract_rule_count: 0,
    ...patch,
  }
}

describe('user contract status', () => {
  it('distinguishes native, active, zero-access and inactive contracts', () => {
    expect(getUserContractStatus(user({}), t).variant).toBe('success')
    expect(
      getUserContractStatus(
        user({ contract_mode: true, contract_version: 1 }),
        t
      ).variant
    ).toBe('danger')
    expect(
      getUserContractStatus(
        user({
          contract_mode: true,
          contract_version: 1,
          contract_rule_count: 3,
        }),
        t
      ).label
    ).toBe('Contract active · {{count}} rules:3')
    expect(
      getUserContractStatus(
        user({ contract_version: 2, contract_rule_count: 4 }),
        t
      ).label
    ).toBe('Contract inactive · {{count}} retained rules:4')
  })
})
