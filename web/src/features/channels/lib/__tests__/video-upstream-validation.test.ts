import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { channelFormSchema } from '../channel-form'

const mediaArraysChannel = {
  name: 'media-arrays-video',
  type: 54,
  base_url: 'http://video.example.com',
  key: 'provider-key',
  models: 'video-model',
  group: ['default'],
  status: 1,
  multi_key_mode: 'single' as const,
  video_upstream_profile: 'third_party_json_video_media_arrays' as const,
  video_upstream_create_path: '/v1/videos',
  video_upstream_query_path_template: '/v1/videos/{task_id}',
  asset_upstream_profile: 'none' as const,
}

describe('video upstream Base URL validation', () => {
  test('accepts an HTTP origin for the media-arrays profile', () => {
    assert.equal(channelFormSchema.safeParse(mediaArraysChannel).success, true)
  })

  test('rejects a path-bearing media-arrays Base URL', () => {
    const result = channelFormSchema.safeParse({
      ...mediaArraysChannel,
      base_url: 'http://video.example.com/provider-root',
    })

    assert.equal(result.success, false)
    if (!result.success) {
      assert.equal(
        result.error.issues.some(
          (issue) =>
            issue.path[0] === 'base_url' &&
            issue.message ===
              'Provide a valid URL starting with http:// or https://'
        ),
        true
      )
    }
  })

  test('accepts an HTTP origin for the FunCloud profile', () => {
    const result = channelFormSchema.safeParse({
      ...mediaArraysChannel,
      video_upstream_profile: 'third_party_funcloud_seedance_v2',
      video_upstream_create_path: '/api/v2/open/aigc/seedance2-0',
      video_upstream_query_path_template: '/api/v2/open/aigc/{task_id}',
    })

    assert.equal(result.success, true)
  })
})
