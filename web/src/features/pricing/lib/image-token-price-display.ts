import type { TFunction } from 'i18next'

import type { DynamicPriceEntry } from './dynamic-price'

/** The exact USD rate accompanies a bounded image quote, before discounts. */
export function imageTokenPriceTitle(
  entry: DynamicPriceEntry,
  t: TFunction
): string | undefined {
  if (
    !entry.variable ||
    entry.unit !== 'token' ||
    entry.formattedRange ||
    !entry.formatted.startsWith('<')
  ) {
    return undefined
  }
  return t('Unit price before discounts: {{price}} USD per 1M tokens.', {
    price: entry.value,
  })
}

/** Image labels describe the configured token categories, never model names. */
export function imageTokenPriceLabel(
  variable: string,
  fallback: string,
  imageGeneration: boolean | undefined,
  prices: Record<string, unknown>
): string {
  if (!imageGeneration) return fallback
  if (variable === 'img') return 'Image Input'
  if (variable === 'img_cr') return 'Cached Image Input'
  if (variable === 'img_o') return 'Image Output'
  if (typeof prices.imagePrice !== 'number') return fallback
  if (variable === 'p') return 'Text Input'
  if (variable === 'cr' && typeof prices.imageCachePrice === 'number') {
    return 'Cached Text Input'
  }
  if (variable === 'c' && typeof prices.imageOutputPrice !== 'number') {
    return 'Image Output'
  }
  return fallback
}

// Price presentation only; stored rates and charged amounts keep full precision.
export const IMAGE_TOKEN_PRICE_FORMAT = {
  digitsLarge: 2,
  digitsSmall: 2,
  minimumFractionDigits: 2,
} as const
