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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createRef } from 'react'
import { describe, expect, test } from 'vitest'

import type { ModelRatioData } from '../model-pricing-core'
import {
  ModelPricingEditorPanel,
  type ModelPricingEditorPanelHandle,
} from '../model-pricing-sheet'

describe('separate Seedance and native task pricing editors', () => {
  test('edits the Seedance pre-consume budget without changing its price or native usage pricing', async () => {
    const user = userEvent.setup()
    const client = new QueryClient()
    const modelName = 'doubao-seedance-2-0-fast-260128'
    const expression =
      'param("_task.has_video_input") == true ? tier("video_input", c * 3.225806451613) : tier("without_video_input", c * 5.425219941349)'
    const nativeExpression = 'tier("base", u("tokens") * 5 / 1000000)'
    // Seed the public API response into the real query cache, preserving the
    // backend contract: the Link entry has no native plugin usage schema.
    client.setQueryData(['status'], { price: 1 })
    client.setQueryData(['pricing'], {
      vendors: [],
      data: [
        {
          model_name: modelName,
          billing_mode: 'tiered_expr',
          billing_expr: expression,
        },
        {
          model_name: 'native-video',
          billing_mode: 'tiered_expr',
          billing_expr: nativeExpression,
          billing_usage_schema: { tokens: { type: 'number', unit: 'token' } },
        },
      ],
    })
    const ref = createRef<ModelPricingEditorPanelHandle>()
    const rendered = render(
      <QueryClientProvider client={client}>
        <ModelPricingEditorPanel
          key={modelName}
          ref={ref}
          editData={{
            name: modelName,
            billingMode: 'tiered_expr',
            billingExpr: expression,
            taskPreConsumeTokens: 325000,
          }}
        />
      </QueryClientProvider>
    )
    await user.click(screen.getAllByRole('combobox')[0])
    await user.click(
      await screen.findByRole('option', { name: 'Visual editor' })
    )
    expect(screen.getByDisplayValue(expression)).toBeInTheDocument()
    expect(
      screen.getByText(
        'This expression cannot be converted to the visual editor without changing its pricing. Continue editing the expression.'
      )
    ).toBeInTheDocument()
    const budget = screen.getByPlaceholderText('250000')
    expect(budget).toHaveValue('325000')
    await user.clear(budget)
    await user.type(budget, '400000')
    await user.tab()
    let saved: ModelRatioData | null = null
    await act(async () => {
      saved = (await ref.current?.commitDraft()) ?? null
      expect(saved).toMatchObject({
        name: modelName,
        billingExpr: expression,
        taskPreConsumeTokens: 400000,
      })
    })

    rendered.rerender(
      <QueryClientProvider client={client}>
        <ModelPricingEditorPanel
          key='native-video'
          ref={ref}
          editData={{
            name: 'native-video',
            billingMode: 'tiered_expr',
            billingExpr: nativeExpression,
          }}
        />
      </QueryClientProvider>
    )
    expect(screen.queryByPlaceholderText('250000')).not.toBeInTheDocument()
    expect(
      screen.getByText(
        'Task usage prices are USD per declared unit. Token fields use dollars per 1M tokens; the editor writes / 1000000 into the expression. Other units are not divided by one million.'
      )
    ).toBeInTheDocument()
    await act(async () => {
      expect(await ref.current?.commitDraft()).toMatchObject({
        name: 'native-video',
        billingExpr: nativeExpression,
      })
    })
    rendered.rerender(
      <QueryClientProvider client={client}>
        <ModelPricingEditorPanel
          key='reopened-seedance'
          ref={ref}
          editData={saved}
        />
      </QueryClientProvider>
    )
    expect(screen.getByPlaceholderText('250000')).toHaveValue('400000')
    expect(screen.getByDisplayValue(expression)).toBeInTheDocument()
    client.clear()
  })
  test('blocks all price editing for a cross-channel contract conflict', async () => {
    const client = new QueryClient()
    client.setQueryData(['status'], { price: 1 })
    client.setQueryData(['pricing'], {
      vendors: [],
      data: [{ model_name: 'conflict', billing_contract_conflict: true }],
    })
    const ref = createRef<ModelPricingEditorPanelHandle>()
    render(
      <QueryClientProvider client={client}>
        <ModelPricingEditorPanel
          ref={ref}
          editData={{
            name: 'conflict',
            billingMode: 'tiered_expr',
            billingExpr: 'tier("base", c * 5)',
            taskPreConsumeTokens: 325000,
          }}
        />
      </QueryClientProvider>
    )
    expect(
      screen.getByText(
        'This model is configured in both Seedance Link and native channels. Use distinct customer model names before editing prices.'
      )
    ).toBeInTheDocument()
    expect(screen.queryByPlaceholderText('250000')).not.toBeInTheDocument()
    await act(async () => {
      expect(await ref.current?.commitDraft()).toBeNull()
    })
    client.clear()
  })

  test('preserves a custom request multiplier on open and rejected visual conversion', async () => {
    const user = userEvent.setup()
    const client = new QueryClient()
    client.setQueryData(['status'], { price: 1 })
    client.setQueryData(['pricing'], { vendors: [], data: [] })
    const ref = createRef<ModelPricingEditorPanelHandle>()
    const billingExpr = 'tier("base", p * 1 + c * 5)'
    const requestRuleExpr = 'param("_task.duration_seconds") * 2'
    render(
      <QueryClientProvider client={client}>
        <ModelPricingEditorPanel
          ref={ref}
          editData={{
            name: 'custom-rules',
            billingMode: 'tiered_expr',
            billingExpr,
            requestRuleExpr,
          }}
        />
      </QueryClientProvider>
    )
    await act(async () => {
      expect(await ref.current?.commitDraft()).toMatchObject({
        billingExpr,
        requestRuleExpr,
      })
    })
    await user.click(screen.getAllByRole('combobox')[0])
    await user.click(
      await screen.findByRole('option', { name: 'Visual editor' })
    )
    await act(async () => {
      expect(await ref.current?.commitDraft()).toMatchObject({
        billingExpr,
        requestRuleExpr,
      })
    })
    client.clear()
  })
  test('retains a supported token price through expression and visual mode', async () => {
    const user = userEvent.setup()
    const client = new QueryClient()
    client.setQueryData(['status'], { price: 1 })
    client.setQueryData(['pricing'], { vendors: [], data: [] })
    const ref = createRef<ModelPricingEditorPanelHandle>()
    const billingExpr = 'tier("base", p * 1 + c * 5)'
    render(
      <QueryClientProvider client={client}>
        <ModelPricingEditorPanel
          ref={ref}
          editData={{
            name: 'token-price',
            billingMode: 'tiered_expr',
            billingExpr,
          }}
        />
      </QueryClientProvider>
    )
    await user.click(screen.getAllByRole('combobox')[0])
    await user.click(
      await screen.findByRole('option', { name: 'Expression editor' })
    )
    await user.click(screen.getAllByRole('combobox')[0])
    await user.click(
      await screen.findByRole('option', { name: 'Visual editor' })
    )
    await act(async () => {
      expect(await ref.current?.commitDraft()).toMatchObject({ billingExpr })
    })
    client.clear()
  })
})
