import { describe, expect, it } from 'vitest'

import {
  CHANNEL_TYPE_MOXING_IMAGE,
  CHANNEL_TYPE_OPTIONS,
  CHANNEL_TYPES,
  MODEL_FETCHABLE_TYPES,
} from '../../constants'
import { getChannelTypeConfig } from '../channel-type-config'

describe('Moxing image channel registration', () => {
  it('registers the dedicated channel next to the strict image relay', () => {
    expect(CHANNEL_TYPE_MOXING_IMAGE).toBe(63)
    expect(CHANNEL_TYPES[CHANNEL_TYPE_MOXING_IMAGE]).toBe('Moxing Image')

    const asyncIndex = CHANNEL_TYPE_OPTIONS.findIndex((option) => option.value === 62)
    const moxingIndex = CHANNEL_TYPE_OPTIONS.findIndex(
      (option) => option.value === CHANNEL_TYPE_MOXING_IMAGE,
    )
    expect(moxingIndex).toBe(asyncIndex + 1)
  })

  it('uses the fixed Moxing endpoint without advertising model discovery', () => {
    const config = getChannelTypeConfig(CHANNEL_TYPE_MOXING_IMAGE)

    expect(config.defaultBaseUrl).toBe('https://www.moxing.pro')
    expect(config.hints?.models).toBe(
      'seedream-5-moxing,seedream-5-pro-moxing',
    )
    expect(MODEL_FETCHABLE_TYPES.has(CHANNEL_TYPE_MOXING_IMAGE)).toBe(false)
  })
})
