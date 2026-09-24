import { describe, expect, test } from 'vitest'

import { CHANNEL_TYPE_OPTIONS } from '../../constants'
import { channelSchema } from '../../types'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformChannelToFormDefaults,
  transformFormDataToUpdatePayload,
} from '../channel-form'
import {
  channelManagementFilter,
  channelManagementTypeOptions,
  minimaxAccessLabel,
} from '../minimax-management'

describe('MiniMax management projection', () => {
  test('a persisted JD channel can be edited and saved without changing its protocol or type', () => {
    const channel = channelSchema.parse({
      id: 138,
      type: 64,
      name: 'fixture',
      key: '',
      models: 'customer',
      status: 1,
      created_time: 0,
      test_time: 0,
      response_time: 0,
      balance_updated_time: 0,
      base_url: 'http://fixture.invalid',
      settings:
        '{"video_upstream_protocol":"jdcloud_video_task_v1","asset_upstream_protocol":"none"}',
    })
    const form = transformChannelToFormDefaults(channel)
    const parsed = channelFormSchema.safeParse({
      ...form,
      minimax_plugin_version: '1.2.0',
    })
    expect(parsed.success).toBe(true)
    if (!parsed.success) return
    const payload = transformFormDataToUpdatePayload(parsed.data, channel.id)
    expect(payload.type).toBe(64)
    expect(JSON.parse(payload.settings ?? '{}').video_upstream_protocol).toBe(
      'jdcloud_video_task_v1'
    )
  })
  test('the create selector has one MiniMax entry without deleting original type definitions', () => {
    const options = channelManagementTypeOptions(CHANNEL_TYPE_OPTIONS)
    expect(
      options
        .filter((item) => item.value === 35 || item.value === 64)
        .map((item) => item.value)
    ).toEqual([35])
    expect(CHANNEL_TYPE_OPTIONS.some((item) => item.value === 64)).toBe(true)
    expect(options.filter((item) => item.value !== 35)).toEqual(
      CHANNEL_TYPE_OPTIONS.filter(
        (item) => item.value !== 35 && item.value !== 64
      )
    )
  })
  test('the group filter requests both types while individual filters retain single-type semantics', () => {
    expect(channelManagementFilter('minimax')).toEqual({ types: '35,64' })
    expect(channelManagementFilter('35')).toEqual({ type: 35 })
    expect(channelManagementFilter('64')).toEqual({ type: 64 })
    expect(channelManagementFilter('all')).toEqual({})
  })
  test('access labels depend on stored identity and do not call unknown protocols JD', () => {
    expect(
      minimaxAccessLabel({
        type: 35,
        settings: '{"video_upstream_protocol":"jdcloud_video_task_v1"}',
      })
    ).toBe('Native API')
    expect(
      minimaxAccessLabel({
        type: 64,
        settings: '{"video_upstream_protocol":"jdcloud_video_task_v1"}',
      })
    ).toBe('JD Cloud · Standard video')
    expect(
      minimaxAccessLabel({
        type: 64,
        settings: '{"video_upstream_protocol":"unknown"}',
      })
    ).toBe('Unrecognized video protocol')
  })
  test('a standard video form cannot save without a declaration version', () => {
    const result = channelFormSchema.safeParse({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      type: 64,
      name: 'test',
      key: 'fixture',
      models: 'customer',
    })
    expect(result.success).toBe(false)
    if (!result.success) {
      expect(
        result.error.issues.some(
          (issue) => issue.path[0] === 'minimax_plugin_version'
        )
      ).toBe(true)
    }
  })
})
