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
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createRef } from 'react'
import { describe, expect, test, vi } from 'vitest'

import { getPricingQueryKey } from '@/features/pricing/hooks/use-pricing-data'

import { TaskUsagePricingEditor } from '../task-usage-pricing-editor'

import type { ModelRatioData } from '../model-pricing-core'
import {
  ModelPricingEditorPanel,
  type ModelPricingEditorPanelHandle,
} from '../model-pricing-sheet'

function getEditorModeSelect() {
  const element = screen
    .getAllByRole('combobox')
    .find((item) => /Visual editor|Expression editor/.test(item.textContent || ''))
  if (!element) throw new Error('Pricing editor mode select was not rendered')
  return element
}

describe('separate Seedance and native task pricing editors', () => {
  test('renders the Seedance task editor with an editable budget when admin data passes the Link schema', async () => {
    // 管理价格接口（props）传入的是 Seedance u() 字段合同，而不是原生插件 schema：
    // 编辑器进入任务用量编辑器，同时独立的预扣预算字段保持可读、可改、可提交。
    const user = userEvent.setup()
    const client = new QueryClient()
    client.setQueryData(['status'], { price: 1 })
    client.setQueryData(getPricingQueryKey(undefined), {
      vendors: [],
      data: [
        {
          model_name: 'seedance-link-model',
          billing_mode: 'tiered_expr',
          billing_expr: 'tier("base", u("tokens") * 5 / 1000000)',
        },
      ],
    })
    const onDirtyChange = vi.fn()
    const ref = createRef<ModelPricingEditorPanelHandle>()
    const rendered = render(
      <QueryClientProvider client={client}>
        <ModelPricingEditorPanel
          key='seedance-link-model'
          ref={ref}
          editData={{
            name: 'seedance-link-model',
            billingMode: 'tiered_expr',
            billingExpr: 'tier("base", u("tokens") * 5 / 1000000)',
            taskPreConsumeTokens: 300000,
          }}
          usageSchema={{
            tokens: { type: 'number', unit: 'token' },
            resolution: { enum: ['480p', '720p', '1080p', '4k'] },
            has_video_input: { type: 'boolean' },
            duration_seconds: { type: 'number', unit: 'second' },
            generate_audio: { type: 'boolean' },
            input_mode: { enum: ['text', 'single_image', 'multi_image', 'multi_modal'] },
            control_mode: { enum: ['none', 'reference', 'end_frame'] },
          }}
          preconsumeTokenBudget
          onDirtyChange={onDirtyChange}
        />
      </QueryClientProvider>
    )
    expect(
      screen.getByText('Async task pre-consume token upper bound')
    ).toBeInTheDocument()
    const budget = screen.getByPlaceholderText('250000')
    expect(budget).toHaveValue('300000')
    await waitFor(() => expect(onDirtyChange).toHaveBeenLastCalledWith(false))
    await user.clear(budget)
    await user.type(budget, '400000')
    await user.tab()
    await waitFor(() => expect(onDirtyChange).toHaveBeenLastCalledWith(true))
    await user.clear(budget)
    await user.type(budget, '300000')
    await user.tab()
    await waitFor(() => expect(onDirtyChange).toHaveBeenLastCalledWith(false))
    await user.clear(budget)
    await user.type(budget, '400000')
    await user.tab()
    await act(async () => {
      await expect(ref.current?.commitDraft()).resolves.toMatchObject({
        name: 'seedance-link-model',
        billingExpr: 'tier("base", u("tokens") * 5 / 1000000)',
        taskPreConsumeTokens: 400000,
      })
    })
    client.clear()
    rendered.unmount()
  })

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
    client.setQueryData(getPricingQueryKey(undefined), {
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
    await user.click(getEditorModeSelect())
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
        /Prices are in .*with units shown below/
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
    client.setQueryData(getPricingQueryKey(undefined), {
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
    client.setQueryData(getPricingQueryKey(undefined), { vendors: [], data: [] })
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
    await user.click(getEditorModeSelect())
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
    client.setQueryData(getPricingQueryKey(undefined), { vendors: [], data: [] })
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
    await user.click(getEditorModeSelect())
    await user.click(
      await screen.findByRole('option', { name: 'Expression editor' })
    )
    await user.click(getEditorModeSelect())
    await user.click(
      await screen.findByRole('option', { name: 'Visual editor' })
    )
    await act(async () => {
      expect(await ref.current?.commitDraft()).toMatchObject({ billingExpr })
    })
    client.clear()
  })
})


describe('missing Seedance budget reminder', () => {
  test.each([
    `tier("base", u("tokens") * 5 / 1000000)`,
    `tier("base", u ( 'tokens' ) * 5 / 1000000)`,
    `tier("fixed", 0.5)`,
  ])('shows a conditional reminder without guessing the contract: %s', (billingExpr) => {
    const props = {
      billingExpr,
      requestRuleExpr: '',
      usageSchema: { tokens: { type: 'number' as const, unit: 'token' as const } },
      showPreconsumeBudget: true,
      onBillingExprChange: vi.fn(),
      onRequestRuleExprChange: vi.fn(),
    }
    const view = render(<TaskUsagePricingEditor {...props} />)
    const message = 'No pre-consume token upper bound is set. Requests that require a token budget will be rejected.'
    expect(screen.getByText(message)).toBeInTheDocument()
    view.rerender(<TaskUsagePricingEditor {...props} taskPreConsumeTokens={300000} />)
    expect(screen.queryByText(message)).not.toBeInTheDocument()
    view.rerender(<TaskUsagePricingEditor {...props} showPreconsumeBudget={false} />)
    expect(screen.queryByText(message)).not.toBeInTheDocument()
    view.unmount()
  })
})
