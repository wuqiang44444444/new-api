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
import { render, waitFor } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { beforeAll, describe, expect, test, vi } from 'vitest'

import type { ModelRatioData } from '../model-pricing-core'

const editorFrames = vi.hoisted(
  () => [] as Array<{ modelName?: string; billingExpr: string }>
)

// Upstream rc.31 panel reads pricing metadata via React Query; stub it so the
// editor remount contract under test needs no QueryClientProvider.
vi.mock('@/features/pricing/hooks/use-pricing-data', () => ({
  usePricingData: () => ({ models: [] }),
}))

vi.mock('../tiered-pricing-editor', () => ({
  TieredPricingEditor: (props: { modelName?: string; billingExpr: string }) => {
    editorFrames.push({
      modelName: props.modelName,
      billingExpr: props.billingExpr,
    })
    return <div data-testid='tiered-pricing-editor'>{props.billingExpr}</div>
  },
}))

const i18n = createInstance()

beforeAll(async () => {
  await i18n.use(initReactI18next).init({
    lng: 'en',
    resources: { en: { translation: {} } },
  })
})

describe('model pricing editor switching', () => {
  test('never renders the previous model expression under the next model name', async () => {
    const expression1080p =
      'v1:tier("base", param("_task.duration_seconds") * 363431.000000)'
    const expression4k =
      'v1:tier("base", param("_task.duration_seconds") * 741114.000000)'
    const model1080p: ModelRatioData = {
      name: 'seedance-2.0-1080p',
      billingMode: 'tiered_expr',
      billingExpr: expression1080p,
    }
    const model4k: ModelRatioData = {
      name: 'seedance-2.0-4k',
      billingMode: 'tiered_expr',
      billingExpr: expression4k,
    }
    const { ModelPricingSheet } = await import('../model-pricing-sheet')
    const renderSheet = (editData: ModelRatioData) => (
      <I18nextProvider i18n={i18n}>
        <ModelPricingSheet
          open
          onOpenChange={() => undefined}
          editData={editData}
        />
      </I18nextProvider>
    )
    const rendered = render(renderSheet(model1080p))

    await waitFor(() => {
      expect(editorFrames.at(-1)).toEqual({
        modelName: model1080p.name,
        billingExpr: expression1080p,
      })
    })

    const switchFrame = editorFrames.length
    rendered.rerender(renderSheet(model4k))

    await waitFor(() => {
      expect(editorFrames.at(-1)).toEqual({
        modelName: model4k.name,
        billingExpr: expression4k,
      })
    })
    expect(editorFrames.slice(switchFrame)).not.toContainEqual({
      modelName: model4k.name,
      billingExpr: expression1080p,
    })
  })
})
