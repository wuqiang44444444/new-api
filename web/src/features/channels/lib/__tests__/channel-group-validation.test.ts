import { describe, expect, test } from 'vitest'

import { CHANNEL_FORM_DEFAULT_VALUES, channelFormSchema } from '../channel-form'
import {
  CHANNEL_GROUP_LENGTH_ERROR,
  isChannelGroupLengthValid,
} from '../channel-group-validation'

describe('channel group list capacity', () => {
  test('accepts 128 characters including comma separators', () => {
    expect(isChannelGroupLengthValid(['a'.repeat(126), 'b'])).toBe(true)
  })

  test('rejects 129 characters including comma separators', () => {
    expect(isChannelGroupLengthValid(['a'.repeat(127), 'b'])).toBe(false)
  })

  test('counts Unicode characters consistently with the database', () => {
    expect(isChannelGroupLengthValid(['组'.repeat(128)])).toBe(true)
    expect(isChannelGroupLengthValid(['😀'.repeat(128)])).toBe(true)
    expect(isChannelGroupLengthValid(['组'.repeat(129)])).toBe(false)
  })

  test('channel form exposes the oversized group error at the group field', () => {
    const result = channelFormSchema.safeParse({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'fixture',
      type: 1,
      key: 'fixture',
      models: 'gpt-4o',
      group: ['a'.repeat(127), 'b'],
    })
    expect(result.success).toBe(false)
    if (!result.success) {
      expect(result.error.issues).toContainEqual(
        expect.objectContaining({
          path: ['group'],
          message: CHANNEL_GROUP_LENGTH_ERROR,
        })
      )
    }
  })
})
