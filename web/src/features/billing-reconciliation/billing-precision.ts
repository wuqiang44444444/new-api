import i18n from '@/i18n/config'

// Customer bill amounts are integer quota in the API. Reject the whole view
// before rendering if JSON numbers cannot represent them precisely; server-side
// CSV generation retains its independent decimal/string path.
export function assertCustomerBillingPrecision(value: unknown): void {
  if (value == null || typeof value !== 'object') return
  for (const [key, child] of Object.entries(value)) {
    if (
      key.endsWith('_quota') &&
      typeof child === 'number' &&
      !Number.isSafeInteger(child)
    ) {
      throw new Error(
        i18n.t('This bill exceeds the precise amount display range.')
      )
    }
    if (child != null && typeof child === 'object') {
      assertCustomerBillingPrecision(child)
    }
  }
}
