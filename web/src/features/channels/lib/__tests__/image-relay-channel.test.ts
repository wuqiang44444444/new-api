import { describe, expect, it } from 'vitest'

import {
  CHANNEL_TYPE_ASYNC_IMAGE,
  CHANNEL_TYPE_SEEDANCE_LINK,
  CHANNEL_TYPES,
  MODEL_FETCHABLE_TYPES,
} from '../../constants'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  transformFormDataToCreatePayload,
} from '../channel-form'
import { getChannelTypeConfig } from '../channel-type-config'

describe('image relay channel registration', () => {
  it('exposes the Image Relay type on 63 next to the Seedance type on 62', () => {
    expect(CHANNEL_TYPE_ASYNC_IMAGE).toBe(63)
    expect(CHANNEL_TYPE_SEEDANCE_LINK).toBe(62)
    expect(CHANNEL_TYPES[CHANNEL_TYPE_ASYNC_IMAGE]).toBe('Image Relay')
    expect(CHANNEL_TYPES[CHANNEL_TYPE_SEEDANCE_LINK]).toBe(
      'Seedance Dedicated Channel'
    )
    expect(MODEL_FETCHABLE_TYPES.has(CHANNEL_TYPE_ASYNC_IMAGE)).toBe(false)
  })

  it('persists the selected protocol independently from model mapping', () => {
    const payload = transformFormDataToCreatePayload({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      type: CHANNEL_TYPE_ASYNC_IMAGE,
      image_upstream_protocol: 'moxing_images_v1',
      model_mapping: '{"customer":"provider"}',
    })
    const settings = JSON.parse(payload.channel.settings || '{}')

    expect(settings.image_upstream_protocol).toBe('moxing_images_v1')
    expect(settings.model_mapping).toBeUndefined()
    expect(getChannelTypeConfig(CHANNEL_TYPE_ASYNC_IMAGE).defaultBaseUrl).toBe(
      'https://mm-internal-cn.leonecloud.com'
    )
  })
})
