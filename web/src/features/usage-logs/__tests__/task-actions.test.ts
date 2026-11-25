import { createInstance } from 'i18next'
import { describe, expect, it } from 'vitest'

import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'

import { taskActionMapper } from '../lib/mappers'

describe('task action display', () => {
  it.each([
    ['image_to_video', 'Image to Video'],
    ['text_to_video', 'Text to Video'],
    ['first_tail_to_video', 'First/Last Frame to Video'],
    ['reference_to_video', 'Reference Video'],
    ['remix', 'Video Remix'],
  ])('displays the API action %s', (action, label) => {
    expect(taskActionMapper.getLabel(action)).toBe(label)
    expect(taskActionMapper.getVariant(action)).toBe('blue')
  })

  it('translates reference generation in task lists and details', async () => {
    const i18n = createInstance()
    await i18n.init({ lng: 'zh', resources: { zh } })

    expect(i18n.t(taskActionMapper.getLabel('reference_to_video'))).toBe(
      '参照生视频'
    )
  })

  it.each([
    ['image_generation', 'Image Generation', '图片生成'],
    ['image_edit', 'Image Editing', '图片编辑'],
  ])(
    'translates the persisted image action %s',
    async (action, english, chinese) => {
      const i18n = createInstance()
      await i18n.init({ lng: 'en', resources: { en, zh } })
      const label = taskActionMapper.getLabel(action)

      expect(i18n.exists(label)).toBe(true)
      expect(i18n.t(label)).toBe(english)
      await i18n.changeLanguage('zh')
      expect(i18n.t(label)).toBe(chinese)
    }
  )
})
