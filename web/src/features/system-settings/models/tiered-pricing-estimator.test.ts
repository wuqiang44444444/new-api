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
import { afterEach, describe, expect, test } from 'vitest'

import { evalExprLocally } from '../../pricing/lib/tier-expr'
import {
  type EstimatorTaskProbe,
  convertRawCost,
  createDefaultDraft,
  loadDraft,
  normalizeDraft,
  probeToRequestBody,
  saveDraft,
} from './tiered-pricing-estimator-state'

const SEEDANCE_EXPR = `param("_task.has_video_input") == true
  ? (param("_task.resolution") == "4k"     ? tier("4k_video",      c * 2.4)
    : param("_task.resolution") == "1080p" ? tier("1080p_video",   c * 4.7)
    :                                       tier("480p720p_video", c * 4.3))
  : (param("_task.resolution") == "4k"     ? tier("4k",            c * 4.0)
    : param("_task.resolution") == "1080p" ? tier("1080p",         c * 7.7)
    :                                       tier("480p720p",       c * 7.0))`

function evalWithProbe(
  expr: string,
  probe: EstimatorTaskProbe,
  completionTokens: number
) {
  return evalExprLocally(
    expr,
    0,
    completionTokens,
    createDefaultDraft().extras,
    { body: probeToRequestBody(probe) }
  )
}

describe('estimator draft normalization', () => {
  test('non-object input falls back to safe defaults', () => {
    const d = normalizeDraft(null)
    expect(d.promptTokens).toBe(0)
    expect(d.taskProbe.hasVideoInput).toBe(false)
    expect(d.taskProbe.resolution).toBe('720p')
  })

  test('negative / NaN / Infinity clamp to 0', () => {
    const d = normalizeDraft({
      promptTokens: -5,
      completionTokens: Number.NaN,
      extras: { cacheReadTokens: Number.POSITIVE_INFINITY },
      taskProbe: { hasVideoInput: true, resolution: '1080p' },
    })
    expect(d.promptTokens).toBe(0)
    expect(d.completionTokens).toBe(0)
    expect(d.extras.cacheReadTokens).toBe(0)
    expect(d.taskProbe.hasVideoInput).toBe(true)
    expect(d.taskProbe.resolution).toBe('1080p')
  })

  test('unknown resolution enum falls back to default', () => {
    const d = normalizeDraft({
      taskProbe: { hasVideoInput: false, resolution: '8k' },
    })
    expect(d.taskProbe.resolution).toBe('720p')
  })

  test('legacy combined resolution migrates to backend 720p contract', () => {
    const d = normalizeDraft({
      taskProbe: { hasVideoInput: false, resolution: '480p720p' },
    })
    expect(d.taskProbe.resolution).toBe('720p')
  })
})

describe('USD / quota conversion (matches backend)', () => {
  test('rawCost -> USD / quota with QuotaPerUnit', () => {
    // 720p/5s 真实 token=108900，命中 480p720p 档 c*7.0 → 与生产实测一致
    const rawCost = 108900 * 7.0
    const { usd, quota } = convertRawCost(rawCost, 500000)
    expect(Math.abs(usd - 0.7623)).toBeLessThan(1e-9)
    expect(quota).toBe(381150)
  })

  test('quota is null when QuotaPerUnit unavailable', () => {
    const { usd, quota } = convertRawCost(770000, 0)
    expect(usd).toBe(0.77)
    expect(quota).toBeNull()
  })
})

describe('_task probe drives correct tier (eight backend values)', () => {
  const cases: Array<{
    name: string
    probe: EstimatorTaskProbe
    expectedTier: string
    unitPrice: number
  }> = [
    {
      name: 'no-video 480p',
      probe: { hasVideoInput: false, resolution: '480p' },
      expectedTier: '480p720p',
      unitPrice: 7.0,
    },
    {
      name: 'no-video 720p',
      probe: { hasVideoInput: false, resolution: '720p' },
      expectedTier: '480p720p',
      unitPrice: 7.0,
    },
    {
      name: 'no-video 1080p',
      probe: { hasVideoInput: false, resolution: '1080p' },
      expectedTier: '1080p',
      unitPrice: 7.7,
    },
    {
      name: 'no-video 4k',
      probe: { hasVideoInput: false, resolution: '4k' },
      expectedTier: '4k',
      unitPrice: 4.0,
    },
    {
      name: 'video 480p',
      probe: { hasVideoInput: true, resolution: '480p' },
      expectedTier: '480p720p_video',
      unitPrice: 4.3,
    },
    {
      name: 'video 720p',
      probe: { hasVideoInput: true, resolution: '720p' },
      expectedTier: '480p720p_video',
      unitPrice: 4.3,
    },
    {
      name: 'video 1080p',
      probe: { hasVideoInput: true, resolution: '1080p' },
      expectedTier: '1080p_video',
      unitPrice: 4.7,
    },
    {
      name: 'video 4k',
      probe: { hasVideoInput: true, resolution: '4k' },
      expectedTier: '4k_video',
      unitPrice: 2.4,
    },
  ]

  for (const { name, probe, expectedTier, unitPrice } of cases) {
    test(name, () => {
      const c = 100000
      const result = evalWithProbe(SEEDANCE_EXPR, probe, c)
      expect(result.matchedTier).toBe(expectedTier)
      expect(result.cost).toBe(c * unitPrice)
      expect(result.error).toBeNull()
    })
  }
})

describe('browser expression safety boundary', () => {
  test('supports versioned billing expressions', () => {
    const result = evalWithProbe(
      'v1:tier("base", c * 2)',
      { hasVideoInput: false, resolution: '720p' },
      10
    )
    expect(result.cost).toBe(20)
    expect(result.error).toBeNull()
  })

  test('rejects browser globals and general JavaScript syntax', () => {
    const globalAccess = evalWithProbe(
      'tier("base", globalThis.fetch("https://example.com"))',
      { hasVideoInput: false, resolution: '720p' },
      10
    )
    expect(globalAccess.error || '').toMatch(/Unsupported identifier/)

    const arrowFunction = evalWithProbe(
      'tier("base", (() => c)())',
      { hasVideoInput: false, resolution: '720p' },
      10
    )
    expect(arrowFunction.error || '').toMatch(/Unsupported JavaScript syntax/)
  })
})

describe('local draft persistence (per model, sanitized on read)', () => {
  const originalWindow = (globalThis as { window?: unknown }).window
  const store = new Map<string, string>()
  const memLocalStorage = {
    getItem: (k: string) => store.get(k) ?? null,
    setItem: (k: string, v: string) => {
      store.set(k, v)
    },
    removeItem: (k: string) => {
      store.delete(k)
    },
  }

  afterEach(() => {
    store.clear()
    if (originalWindow === undefined) {
      delete (globalThis as { window?: unknown }).window
    } else {
      ;(globalThis as { window?: unknown }).window = originalWindow
    }
  })

  test('writes and restores a per-model draft', () => {
    ;(globalThis as { window?: unknown }).window = {
      localStorage: memLocalStorage,
    } as unknown as typeof globalThis.window

    saveDraft('seedance-2-0-oversea', {
      promptTokens: 0,
      completionTokens: 250000,
      extras: createDefaultDraft().extras,
      taskProbe: { hasVideoInput: false, resolution: '1080p' },
    })
    const restored = loadDraft('seedance-2-0-oversea')
    expect(restored.completionTokens).toBe(250000)
    expect(restored.taskProbe.resolution).toBe('1080p')
  })

  test('corrupt JSON in storage falls back to defaults', () => {
    ;(globalThis as { window?: unknown }).window = {
      localStorage: memLocalStorage,
    } as unknown as typeof globalThis.window
    store.set('model-pricing-estimator:v1:bad', '{not json')
    const restored = loadDraft('bad')
    expect(restored).toStrictEqual(createDefaultDraft())
  })

  test('empty model name does not touch storage', () => {
    ;(globalThis as { window?: unknown }).window = {
      localStorage: memLocalStorage,
    } as unknown as typeof globalThis.window
    saveDraft('', {
      ...createDefaultDraft(),
      completionTokens: 999,
    })
    expect(store.size).toBe(0)
    expect(loadDraft('')).toStrictEqual(createDefaultDraft())
  })
})
