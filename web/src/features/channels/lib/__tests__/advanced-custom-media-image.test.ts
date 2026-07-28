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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  getAdvancedCustomConverterDefaults,
  getAdvancedCustomIncomingPathOptions,
  getAdvancedCustomTemplateConfig,
  isAdvancedCustomIncomingPathAllowed,
  validateAdvancedCustomConfig,
} from '../advanced-custom'

describe('media task image advanced custom converter', () => {
  test('is limited to the OpenAI image generation route', () => {
    assert.equal(
      isAdvancedCustomIncomingPathAllowed(
        '/v1/images/generations',
        'media_task_image_blocking'
      ),
      true
    )
    assert.equal(
      isAdvancedCustomIncomingPathAllowed(
        '/v1/chat/completions',
        'media_task_image_blocking'
      ),
      false
    )
    assert.deepEqual(
      getAdvancedCustomIncomingPathOptions('media_task_image_blocking').map(
        (option) => option.value
      ),
      ['/v1/images/generations']
    )
  })

  test('uses a bearer-authenticated image route by default', () => {
    assert.deepEqual(
      getAdvancedCustomConverterDefaults(
        'media_task_image_blocking',
        '/v1/images/generations'
      ),
      {
        upstream_path: '/v1/images/generations',
        auth: {
          type: 'header',
          name: 'Authorization',
          value: 'Bearer {api_key}',
        },
      }
    )
  })

  test('provides a valid template scoped to the supported upstream models', () => {
    const config = getAdvancedCustomTemplateConfig('tokensave_moxing_images')

    assert.equal(validateAdvancedCustomConfig(config), null)
    assert.deepEqual(config.advanced_routes, [
      {
        incoming_path: '/v1/images/generations',
        upstream_path: '/v1/images/generations',
        converter: 'media_task_image_blocking',
        models: ['seedream-5-moxing'],
        auth: {
          type: 'header',
          name: 'Authorization',
          value: 'Bearer {api_key}',
        },
      },
      {
        incoming_path: '/v1/images/generations',
        upstream_path: '/v1/media/generations',
        converter: 'media_task_image_blocking',
        models: ['nano-banana-2'],
        auth: {
          type: 'header',
          name: 'Authorization',
          value: 'Bearer {api_key}',
        },
      },
    ])
  })
})
