import { expect, it } from 'vitest'

import { taskStatusMapper } from '../mappers'

it.each([
  ['CANCELLED', 'Cancelled'],
  ['EXPIRED', 'Expired'],
  ['PROVIDER_CONTRACT_FAILURE', 'Upstream response invalid'],
])('shows a human-readable terminal state for %s', (state, label) => {
  expect(taskStatusMapper.getLabel(state, state)).toBe(label)
})
