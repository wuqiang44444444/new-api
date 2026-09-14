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
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createJSONStorage } from 'zustand/middleware'

import { api } from '@/lib/api'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { ModelCard } from '../components/model-card'
import { ModelCardGrid } from '../components/model-card-grid'
import type { PricingModel } from '../types'
import {
  billingDisplayFixture,
  taskBillingDisplayFixture,
} from './billing-display-fixtures'

function pricingModel(overrides: Partial<PricingModel> = {}): PricingModel {
  return {
    id: 1,
    model_name: 'example-model',
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 3,
    enable_groups: ['default', 'premium'],
    group_ratio: { default: 1, premium: 3 },
    billing_display: billingDisplayFixture(overrides.billing_expr),
    ...overrides,
  }
}

let queryClient: QueryClient
const originalStorage = useSystemConfigStore.persist.getOptions().storage
beforeEach(() => {
  const storage = new Map<string, string>()
  vi.stubGlobal('localStorage', {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, value: string) => storage.set(key, value),
    removeItem: (key: string) => storage.delete(key),
    clear: () => storage.clear(),
    key: (index: number) => [...storage.keys()][index] ?? null,
    get length() {
      return storage.size
    },
  })
  useSystemConfigStore.persist.setOptions({
    storage: createJSONStorage(() => localStorage),
  })
  useSystemConfigStore.getState().setConfig({
    currency: { ...DEFAULT_CURRENCY_CONFIG, quotaDisplayType: 'USD' },
  })
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
})
afterEach(() => {
  queryClient.clear()
  useSystemConfigStore
    .getState()
    .setConfig({ currency: { ...DEFAULT_CURRENCY_CONFIG } })
  vi.useRealTimers()
  vi.unstubAllGlobals()
  useSystemConfigStore.persist.setOptions({ storage: originalStorage })
})

describe('model cards', () => {
  it('copies the complete long model name without opening details', async () => {
    const user = userEvent.setup()
    const onClick = vi.fn()
    const name = 'provider/model-with-a-long-name-and-a-version-suffix-20260906'
    render(
      <ModelCard model={pricingModel({ model_name: name })} onClick={onClick} />
    )
    expect(screen.getByRole('heading', { name })).toHaveAttribute('title', name)
    await user.click(screen.getByRole('button', { name: 'Copy model name' }))
    expect(await navigator.clipboard.readText()).toBe(name)
    expect(onClick).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Details' }))
    expect(onClick).toHaveBeenCalledOnce()
  })

  it('omits empty description and unavailable metrics while retaining details', () => {
    render(<ModelCard model={pricingModel()} onClick={vi.fn()} />)
    expect(
      screen.queryByLabelText('Performance metrics for the last 24 hours')
    ).not.toBeInTheDocument()
    expect(
      screen.queryByText('No description available.')
    ).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Details' })).toBeEnabled()
  })

  it('keeps asset sharing outside the decorative icon and allows its row to wrap', () => {
    render(
      <ModelCard
        model={pricingModel({
          model_name: 'seedance-with-a-long-customer-model-name',
          asset_share_group: {
            label: '8018',
            models: [
              'seedance-with-a-long-customer-model-name',
              'another-model',
            ],
          },
        })}
        onClick={vi.fn()}
      />
    )
    const badge = screen.getByText('Asset share group 8018')
    expect(badge.closest('[aria-hidden="true"]')).toBeNull()
    expect(badge.closest('[tabindex="0"]')).toHaveClass('max-w-full')
    expect(badge.closest('[tabindex="0"]')?.parentElement).toHaveClass(
      'flex-wrap'
    )
    expect(screen.getByRole('heading')).toHaveClass('[overflow-wrap:anywhere]')
  })

  it('reveals the complete asset sharing label and models on keyboard focus', async () => {
    const user = userEvent.setup()
    render(
      <ModelCard
        model={pricingModel({
          asset_share_group: {
            label: '8018',
            models: ['first-model', 'second-model'],
          },
        })}
        onClick={vi.fn()}
      />
    )
    await user.tab()
    expect(await screen.findByText('first-model, second-model')).toBeVisible()
    expect(screen.getAllByText('Asset share group 8018')).toHaveLength(2)
  })

  it('keeps group, endpoint and tag overflow counts with their own metadata', () => {
    const groups = ['default-with-a-long-group-name', 'premium', 'internal']
    const endpoints = ['openai-response', 'openai', 'claude', 'gemini', 'jina']
    const tags = [
      'video-generation',
      'high-resolution',
      'fast',
      'batch',
      'hd',
      'pro',
    ]
    render(
      <ModelCard
        model={pricingModel({
          enable_groups: groups,
          supported_endpoint_types: endpoints,
          tags: tags.join(','),
        })}
        onClick={vi.fn()}
      />
    )

    const groupField = screen.getByText('Groups').parentElement
    const endpointField = screen.getByText('Endpoints').parentElement
    if (!groupField || !endpointField) {
      throw new Error('Expected labeled group and endpoint fields')
    }
    expect(groupField.parentElement).toHaveClass(
      'grid-cols-1',
      '@min-[360px]:grid-cols-2'
    )
    const tagField = screen.getByRole('group', { name: 'Tags' })
    expect(within(groupField).getByText(groups[0])).toBeVisible()
    expect(within(groupField).getByText('+2')).toHaveAttribute(
      'title',
      groups.slice(1).join(', ')
    )
    expect(
      within(endpointField).getByText('openai-response, openai')
    ).toHaveAttribute('title', endpoints.join(', '))
    expect(within(endpointField).getByText('+3')).toBeVisible()
    expect(
      within(tagField).getByText('video-generation, high-resolution')
    ).toHaveAttribute('title', tags.join(', '))
    expect(within(tagField).getByText('+4')).toBeVisible()
    expect(
      within(screen.getByRole('group', { name: 'Pricing' })).getByText(
        'Token-based'
      )
    ).toBeVisible()
  })

  it('omits metadata fields when the model has no groups, endpoints or tags', () => {
    render(
      <ModelCard
        model={pricingModel({ enable_groups: [] })}
        onClick={vi.fn()}
      />
    )
    expect(screen.queryByText('Groups')).not.toBeInTheDocument()
    expect(screen.queryByText('Endpoints')).not.toBeInTheDocument()
    expect(
      screen.queryByRole('group', { name: 'Tags' })
    ).not.toBeInTheDocument()
  })

  it.each([
    { success_rate: 0, expected: '0.0%' },
    { success_rate: 99.8, expected: '99.8%' },
    { success_rate: Number.NaN, expected: '—%' },
  ])(
    'shows $expected for the reported request success rate $success_rate',
    ({ success_rate, expected }) => {
      render(
        <ModelCard
          model={pricingModel()}
          onClick={vi.fn()}
          perf={{ avg_latency_ms: 1200, avg_tps: 42, success_rate }}
        />
      )
      const metrics = screen.getByLabelText(
        'Performance metrics for the last 24 hours'
      )
      expect(within(metrics).getByText(expected)).toBeVisible()
      expect(within(metrics).getByText('Status')).toBeVisible()
      expect(within(metrics).getByText('1.20s')).toBeVisible()
      expect(within(metrics).getByText('42.0t/s')).toBeVisible()
    }
  )

  it('keeps group and recharge pricing correct when changing the token unit, including a free cache price', () => {
    const props = {
      model: pricingModel({ cache_ratio: 0 }),
      onClick: vi.fn(),
      selectedGroup: 'premium',
      showRechargePrice: true,
      priceRate: 3,
      usdExchangeRate: 6,
    }
    const { rerender } = render(<ModelCard {...props} tokenUnit='M' />)
    expect(screen.getByText('Input').parentElement).toHaveTextContent(
      /\$3\s*\/\s*1M/
    )
    expect(screen.getByText('Output').parentElement).toHaveTextContent(
      /\$9\s*\/\s*1M/
    )
    expect(screen.getByText('Cached').parentElement).toHaveTextContent(
      /\$0\s*\/\s*1M/
    )
    rerender(<ModelCard {...props} tokenUnit='K' />)
    expect(screen.getByText('Input').parentElement).toHaveTextContent(
      /\$0.003\s*\/\s*1K/
    )
    expect(screen.getByText('Output').parentElement).toHaveTextContent(
      /\$0.009\s*\/\s*1K/
    )
    expect(screen.getByText('Cached').parentElement).toHaveTextContent(
      /\$0\s*\/\s*1K/
    )
  })

  it('shows a per-request price with the selected group and recharge multiplier without a token unit', () => {
    render(
      <ModelCard
        model={pricingModel({ quota_type: 1, model_price: 0.4 })}
        onClick={vi.fn()}
        selectedGroup='premium'
        showRechargePrice
        priceRate={3}
        usdExchangeRate={6}
        tokenUnit='K'
      />
    )
    expect(screen.getByText(/\$0.6/)).toHaveTextContent(/\$0.6\s*\/\s*request/)
    expect(screen.queryByText(/1K|1M/)).not.toBeInTheDocument()
    expect(screen.getAllByText('Per Request')).toHaveLength(1)
    expect(screen.queryByText('Per-request')).not.toBeInTheDocument()
  })

  it('preserves expression prices and makes the selected token unit explicit', () => {
    render(
      <ModelCard
        model={pricingModel({
          billing_mode: 'tiered_expr',
          billing_expr: 'tier("base", p * 3 + c * 15)',
        })}
        onClick={vi.fn()}
        tokenUnit='K'
        selectedGroup='premium'
        showRechargePrice
        priceRate={3}
        usdExchangeRate={6}
      />
    )
    expect(screen.getByText('Input').parentElement).toHaveTextContent(
      /\$0.0045\s*\/\s*1K/
    )
    expect(screen.getByText('Output').parentElement).toHaveTextContent(
      /\$0.0225\s*\/\s*1K/
    )
  })

  it('keeps task price ranges in their actual usage unit instead of the selected token unit', () => {
    render(
      <ModelCard
        model={pricingModel({
          billing_mode: 'tiered_expr',
          billing_expr:
            'u("mode") == "pro" ? tier("pro", u("seconds") * 0.8) : tier("std", u("seconds") * 0.4)',
          billing_display: taskBillingDisplayFixture(
            'u("mode") == "pro" ? tier("pro", u("seconds") * 0.8) : tier("std", u("seconds") * 0.4)'
          ),
          billing_usage_schema: {
            seconds: { type: 'number', unit: 'second' },
            mode: { enum: ['std', 'pro'] },
          },
        })}
        onClick={vi.fn()}
        tokenUnit='K'
      />
    )
    expect(screen.getByText(/0.4.*0.8/)).toHaveTextContent(/0.4 – \$0.8/)
    expect(screen.getByText(/^\/\s*s$/)).toBeVisible()
    expect(screen.queryByText(/1K|1M/)).not.toBeInTheDocument()
  })

  it('expands the resolution matrix formula into a ranged card summary', () => {
    // 线上截图公式形状:tier() 内部按分辨率选择单价,外层按是否带参考视频分档。
    const expression =
      'u("has_video_input") ? tier("base", u("resolution") == "1080p" ? u("tokens") * 6.762 / 1000000 : u("tokens") * 6.174 / 1000000) : tier("base", u("resolution") == "1080p" ? u("tokens") * 11.319 / 1000000 : u("tokens") * 10.29 / 1000000)'
    render(
      <ModelCard
        model={pricingModel({
          billing_mode: 'tiered_expr',
          billing_expr: expression,
          billing_display: taskBillingDisplayFixture(expression),
          billing_usage_schema: {
            tokens: { type: 'number', unit: 'token' },
            resolution: { enum: ['1080p'] },
            has_video_input: { type: 'boolean' },
          },
        })}
        onClick={vi.fn()}
        tokenUnit='K'
      />
    )
    // 卡片显示跨全部条件档位的范围,不显示第一档价格,也不显示原式。
    expect(screen.getByText(/\$6\.174 – \$11\.319/)).toBeVisible()
    expect(screen.queryByText(/u\("has_video_input"\)/)).not.toBeInTheDocument()
  })

  it('shows every resolution rate when the saved formula multiplies usage by a conditional price', () => {
    // Use the saved multiplicative formula, not an algebraically rewritten fixture.
    const expression =
      'u("has_video_input") ? tier("reference_video", u("tokens") * (u("resolution") == "4k" ? 2.352 : u("resolution") == "1080p" ? 4.557 : 4.116) / 1000000) : tier("no_video", u("tokens") * (u("resolution") == "4k" ? 3.822 : u("resolution") == "1080p" ? 7.497 : 6.762) / 1000000)'
    render(
      <ModelCard
        model={pricingModel({
          billing_mode: 'tiered_expr',
          billing_expr: expression,
          billing_display: taskBillingDisplayFixture(expression),
          billing_usage_schema: {
            tokens: { type: 'number', unit: 'token' },
            resolution: { enum: ['480p', '720p', '1080p', '4k'] },
            has_video_input: { type: 'boolean' },
          },
        })}
        onClick={vi.fn()}
        tokenUnit='K'
      />
    )
    // 卡片显示跨全部条件档位的范围,不显示第一档价格,也不显示原式。
    expect(screen.getByText(/\$2\.352 – \$7\.497/)).toBeVisible()
    expect(screen.queryByText(/u\("has_video_input"\)/)).not.toBeInTheDocument()
  })

  it('shows the unconfigured usage message without inventing a token price', () => {
    render(
      <ModelCard
        model={pricingModel({
          billing_usage_schema: { seconds: { type: 'number', unit: 'second' } },
        })}
        onClick={vi.fn()}
      />
    )
    expect(
      screen.getByText('Usage-based billing · price not configured')
    ).toBeVisible()
    expect(screen.queryByText('Input')).not.toBeInTheDocument()
  })

  it('shows a spaced task token range with its unit when an example price is present', () => {
    render(
      <ModelCard
        model={pricingModel({
          billing_mode: 'tiered_expr',
          billing_expr:
            'u("mode") == "pro" ? tier("pro", u("tokens") * 70 / 1000000) : tier("std", u("tokens") * 42 / 1000000)',
          billing_display: taskBillingDisplayFixture(
            'u("mode") == "pro" ? tier("pro", u("tokens") * 70 / 1000000) : tier("std", u("tokens") * 42 / 1000000)'
          ),
          billing_usage_schema: {
            tokens: { type: 'number', unit: 'token' },
            mode: { enum: ['std', 'pro'] },
          },
          billing_usage_examples: [
            // total 由后端求值：42/1e6 × 48000。
            {
              label: '480p · 5s',
              facts: { tokens: 48000, mode: 'std' },
              total: 2.016,
            },
          ],
        })}
        onClick={vi.fn()}
        tokenUnit='K'
      />
    )
    expect(screen.getByText('$42 – $70').parentElement).toHaveTextContent(
      '$42 – $70 / 1M token'
    )
    expect(screen.getByText(/480p · 5s ≈/)).toBeVisible()
  })

  it('keeps an unrecognized expression unexpanded with the special billing message', () => {
    const expression =
      'u("seconds") > 30 ? tier("long", u("seconds") * 0.3) : tier("short", u("seconds") * 0.4)'
    render(
      <ModelCard
        model={pricingModel({
          billing_mode: 'tiered_expr',
          billing_expr: expression,
          billing_usage_schema: { seconds: { type: 'number', unit: 'second' } },
        })}
        onClick={vi.fn()}
      />
    )
    expect(screen.getByText('Special billing expression')).toBeVisible()
    // 无法证明的投影保持不可展开状态；原式不作为价格展示给客户。
    expect(
      screen.getByText('Pricing details temporarily unavailable')
    ).toBeVisible()
    expect(screen.queryByText(expression)).not.toBeInTheDocument()
  })

  it('keeps browsing available without empty metrics after the metrics request fails', async () => {
    const request = vi
      .spyOn(api, 'get')
      .mockRejectedValue(new Error('metrics unavailable'))
    const onModelClick = vi.fn()
    render(
      <QueryClientProvider client={queryClient}>
        <ModelCardGrid models={[pricingModel()]} onModelClick={onModelClick} />
      </QueryClientProvider>
    )
    await waitFor(() =>
      expect(
        queryClient.getQueryState(['perf-metrics-summary', 24])?.status
      ).toBe('error')
    )
    expect(request).toHaveBeenCalledWith('/api/perf-metrics/summary', {
      params: { hours: 24 },
    })
    expect(
      screen.queryByLabelText('Performance metrics for the last 24 hours')
    ).not.toBeInTheDocument()
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Details' }))
    expect(onModelClick).toHaveBeenCalledWith('example-model')
  })

  it('paginates the model cards and disables navigation at both boundaries', async () => {
    queryClient.setQueryData(['perf-metrics-summary', 24], {
      success: true,
      data: { models: [] },
    })
    const models = Array.from({ length: 21 }, (_, index) =>
      pricingModel({ id: index + 1, model_name: `model-${index + 1}` })
    )
    const user = userEvent.setup()
    render(
      <QueryClientProvider client={queryClient}>
        <ModelCardGrid models={models} onModelClick={vi.fn()} />
      </QueryClientProvider>
    )
    expect(screen.getByRole('button', { name: 'Previous page' })).toBeDisabled()
    expect(screen.getAllByRole('heading', { level: 3 })).toHaveLength(20)
    await user.click(screen.getByRole('button', { name: 'Next page' }))
    expect(screen.getAllByRole('heading', { level: 3 })).toHaveLength(1)
    expect(screen.getByRole('heading', { name: 'model-21' })).toBeVisible()
    expect(screen.getByRole('button', { name: 'Next page' })).toBeDisabled()
    await user.click(screen.getByRole('button', { name: 'Previous page' }))
    expect(screen.getByRole('heading', { name: 'model-1' })).toBeVisible()
  })

  it('switches the card grid to three columns at the xl breakpoint instead of 2xl', () => {
    queryClient.setQueryData(['perf-metrics-summary', 24], {
      success: true,
      data: { models: [] },
    })
    render(
      <QueryClientProvider client={queryClient}>
        <ModelCardGrid models={[pricingModel()]} onModelClick={vi.fn()} />
      </QueryClientProvider>
    )
    const grid = screen
      .getByRole('heading', { name: 'example-model' })
      .closest('.grid')
    expect(grid).toHaveClass('xl:grid-cols-3')
    expect(grid).not.toHaveClass('2xl:grid-cols-3')
    expect(grid).not.toHaveClass('min-[1440px]:grid-cols-3')
  })

  it('lights slots 23 and 18 when series has the current hour and five hours earlier', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-09-07T12:00:00.000Z'))
    const currentHourStart = Math.floor(Date.now() / 1000 / 3600) * 3600

    render(
      <ModelCard
        model={pricingModel()}
        onClick={vi.fn()}
        perf={{
          avg_latency_ms: 1200,
          avg_tps: 42,
          success_rate: 100,
          recent_success_series: [
            { ts: currentHourStart, success_rate: 100 },
            { ts: currentHourStart - 5 * 3600, success_rate: 80 },
          ],
        }}
      />
    )

    const spans = [
      ...screen.getByRole('img', {
        name: 'Recent success-rate samples; gray bars indicate missing data.',
      }).children,
    ]
    expect(spans).toHaveLength(24)
    spans.forEach((slot, index) => {
      if (index === 18 || index === 23) {
        expect(slot.classList.contains('bg-muted-foreground/15')).toBe(false)
        return
      }
      expect(slot.classList.contains('bg-muted-foreground/15')).toBe(true)
    })
    vi.useRealTimers()
  })

  it('keeps all 24 slots gray when a series point is 24 hours before the current hour', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-09-07T12:00:00.000Z'))
    const currentHourStart = Math.floor(Date.now() / 1000 / 3600) * 3600

    render(
      <ModelCard
        model={pricingModel()}
        onClick={vi.fn()}
        perf={{
          avg_latency_ms: 1200,
          avg_tps: 42,
          success_rate: 100,
          recent_success_series: [
            { ts: currentHourStart - 24 * 3600, success_rate: 100 },
          ],
        }}
      />
    )

    const spans = [
      ...screen.getByRole('img', {
        name: 'Recent success-rate samples; gray bars indicate missing data.',
      }).children,
    ]
    expect(spans).toHaveLength(24)
    spans.forEach((slot) => {
      expect(slot.classList.contains('bg-muted-foreground/15')).toBe(true)
    })
    vi.useRealTimers()
  })

  it('keeps all 24 slots gray when recent_success_series is undefined', () => {
    render(
      <ModelCard
        model={pricingModel()}
        onClick={vi.fn()}
        perf={{ avg_latency_ms: 1200, avg_tps: 42, success_rate: 100 }}
      />
    )

    const spans = [
      ...screen.getByRole('img', {
        name: 'Recent success-rate samples; gray bars indicate missing data.',
      }).children,
    ]
    expect(spans).toHaveLength(24)
    spans.forEach((slot) => {
      expect(slot.classList.contains('bg-muted-foreground/15')).toBe(true)
    })
  })

  it('places a five-hour-old point in slot 18 when now is mid-hour', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-09-07T12:37:00.000Z'))
    const currentHourStart = Math.floor(Date.now() / 1000 / 3600) * 3600

    render(
      <ModelCard
        model={pricingModel()}
        onClick={vi.fn()}
        perf={{
          avg_latency_ms: 1200,
          avg_tps: 42,
          success_rate: 80,
          recent_success_series: [
            { ts: currentHourStart - 5 * 3600, success_rate: 80 },
          ],
        }}
      />
    )

    const spans = [
      ...screen.getByRole('img', {
        name: 'Recent success-rate samples; gray bars indicate missing data.',
      }).children,
    ]
    expect(spans).toHaveLength(24)
    spans.forEach((slot, index) => {
      if (index === 18) {
        expect(slot.classList.contains('bg-muted-foreground/15')).toBe(false)
        return
      }
      expect(slot.classList.contains('bg-muted-foreground/15')).toBe(true)
    })
    vi.useRealTimers()
  })
})
