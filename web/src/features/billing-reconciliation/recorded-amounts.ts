import type { CustomerStatement } from './types'

// Older servers return reverse-calculated amounts under the same amount keys.
export function statementWithRecordedAmounts(
  statement: CustomerStatement | undefined
): CustomerStatement | undefined {
  if (!statement || statement.recorded_original_amounts) return statement

  const clearAmounts = { original_quota: undefined, discount_quota: undefined }
  return {
    ...statement,
    ...clearAmounts,
    groups: statement.groups.map((group) => ({
      ...group,
      ...clearAmounts,
      models: group.models.map((model) => ({ ...model, ...clearAmounts })),
    })),
    discount_combinations: statement.discount_combinations?.map((row) => ({
      ...row,
      ...clearAmounts,
      original_known: false,
    })),
  }
}
