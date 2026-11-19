import { t } from 'i18next'
import { toast } from 'sonner'

export type ImagePriceWarning = { code: string; field?: string }

// The expression alone cannot establish which dimensions a model accepts.
export function hasImagePricingIssue(warnings: ImagePriceWarning[]) {
  return warnings.some((warning) => warning.code !== 'check_pixel_size_prices')
}

export function imagePriceWarningText(warning: ImagePriceWarning) {
  switch (warning.code) {
    case 'token_input':
      return t('Per-image pricing uses token input {{field}}.', {
        field: warning.field,
      })
    case 'unavailable_parameter':
      return t(
        'The native per-image billing input does not include {{field}}.',
        { field: warning.field }
      )
    case 'conflicting_tier':
      return t('Tier {{field}} has different prices.', {
        field: warning.field,
      })
    case 'check_pixel_size_prices':
      return t(
        'If this model accepts pixel dimensions, check prices for sizes such as 3840x2160.'
      )
    default:
      return warning.code
  }
}

export function showSavedImagePriceWarnings(
  warnings?: Record<string, ImagePriceWarning[]>
) {
  for (const [model, issues] of Object.entries(warnings ?? {})) {
    const notify = hasImagePricingIssue(issues) ? toast.warning : toast.info
    notify(model, {
      description: issues.map(imagePriceWarningText).join('\n'),
    })
  }
}
