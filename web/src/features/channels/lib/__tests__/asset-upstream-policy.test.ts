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

import { describe, test } from 'vitest'

import {
  assetMinURLTTLWithDefault,
  DEFAULT_ASSET_MIN_URL_TTL_SECONDS,
} from '../asset-upstream-policy'

describe('asset upstream URL TTL policy', () => {
  test('uses one hour when an enabled asset profile has no positive value', () => {
    assert.equal(
      assetMinURLTTLWithDefault('relay_assets', 0),
      DEFAULT_ASSET_MIN_URL_TTL_SECONDS
    )
  })

  test('preserves an explicitly configured positive value', () => {
    assert.equal(assetMinURLTTLWithDefault('relay_assets', 7200), 7200)
  })

  test('does not add a TTL when the asset protocol is disabled', () => {
    assert.equal(assetMinURLTTLWithDefault('none', 0), 0)
  })
})
