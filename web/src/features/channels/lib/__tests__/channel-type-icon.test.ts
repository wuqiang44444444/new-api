/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { describe, expect, test } from 'vitest'

import {
  CHANNEL_TYPE_ASYNC_IMAGE,
  CHANNEL_TYPE_SEEDANCE_LINK,
  CHANNEL_TYPE_TASK_PLUGIN,
} from '../../constants'
import { getChannelTypeIcon } from '../channel-utils'

describe('channel type icon metadata', () => {
  test.each([
    [CHANNEL_TYPE_TASK_PLUGIN, 'NewAPI'],
    [CHANNEL_TYPE_SEEDANCE_LINK, 'Doubao'],
    [CHANNEL_TYPE_ASYNC_IMAGE, 'OpenAI'],
  ])('maps channel type %i to %s', (channelType, icon) => {
    expect(getChannelTypeIcon(channelType)).toBe(icon)
  })
})
