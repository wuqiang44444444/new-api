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
import { cleanup, render, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { ModelCard } from '../components/model-card'
import { ModelDetailsContent } from '../components/model-details'
import { ModelPriceCell } from '../components/model-price-cell'
import { getDynamicPricingSummary } from '../lib/dynamic-price'
import type { PricingModel } from '../types'
import {
  billingDisplayFixture,
  taskBillingDisplayFixture,
} from './billing-display-fixtures'

vi.mock('@visactor/react-vchart', () => ({ VChart: () => null }))

const nativeExpression =
  'len <= 100 ? tier("short", p * 2 + c * 8) : tier("long", p * 4 + c * 16 + img_o * 10 + 1000000)'
const taskExpression =
  'u("generate_audio") ? tier("audio", 1 + u("duration_seconds") * 0.8 + u("clips") * 0.2 + u("tokens") * 2 / 1000000) : tier("silent", u("duration_seconds") * 0.4)'

function pricingModel(overrides: Partial<PricingModel> = {}): PricingModel {
  return {
    id: 1,
    model_name: 'contract-display',
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 1,
    enable_groups: ['default'],
    ...overrides,
  }
}

const conditionalModels = [
  pricingModel({
    billing_mode: 'tiered_expr',
    billing_expr: nativeExpression,
    billing_display: billingDisplayFixture(nativeExpression),
  }),
  pricingModel({
    billing_mode: 'tiered_expr',
    billing_expr: taskExpression,
    billing_display: taskBillingDisplayFixture(taskExpression),
    billing_usage_schema: {
      duration_seconds: { type: 'number', unit: 'second' },
      clips: { type: 'number', unit: 'count' },
      tokens: { type: 'number', unit: 'token' },
      generate_audio: { type: 'boolean' },
    },
  }),
]

const clients: QueryClient[] = []
afterEach(() => {
  cleanup()
  clients.forEach((client) => client.clear())
  clients.length = 0
})

describe('conditional price components', () => {
  it.each([
    {
      expression:
        '(hour("Asia/Shanghai") >= 9 ? 2 : 4) * tier("base", p * 2 + 100000)',
      task: false,
      expected: '$4 – $8',
      fixed: '$0.2 – $0.4',
    },
    {
      expression:
        '(hour("Asia/Shanghai") >= 9 ? 2 : 2) * tier("base", p * 2 + 100000)',
      task: false,
      expected: '$4',
      fixed: '$0.2',
    },
    {
      expression:
        '(u("generate_audio") ? 0.5 : 0.25) * tier("base", u("duration_seconds") * 0.4 + 1)',
      task: true,
      expected: '$0.1 – $0.2',
      fixed: '$0.25 – $0.5',
    },
  ])(
    'only advertises actual scenario prices for $expression',
    ({ expression, task, expected, fixed }) => {
      const model = pricingModel({
        billing_mode: 'tiered_expr',
        billing_expr: expression,
        billing_display: task
          ? taskBillingDisplayFixture(expression)
          : billingDisplayFixture(expression),
        billing_usage_schema: task
          ? {
              duration_seconds: { type: 'number', unit: 'second' },
              generate_audio: { type: 'boolean' },
            }
          : undefined,
      })
      const summary = getDynamicPricingSummary(model, { tokenUnit: 'M' })
      const usage = summary?.primaryEntries.find(
        (entry) => entry.unit !== 'request'
      )
      const constant = summary?.primaryEntries.find(
        (entry) => entry.unit === 'request'
      )
      expect(usage?.formattedRange ?? usage?.formatted).toBe(expected)
      expect(constant?.formattedRange ?? constant?.formatted).toBe(fixed)
      render(<ModelCard model={model} onClick={vi.fn()} tokenUnit='M' />)
      expect(screen.getByText(expected)).toBeInTheDocument()
      expect(screen.getByText(fixed)).toBeInTheDocument()
    }
  )

  it.each(conditionalModels)(
    'keeps every projected component in summaries for $billing_expr',
    (model) => {
      const before = structuredClone(model.billing_display)
      const summary = getDynamicPricingSummary(model, {
        tokenUnit: 'K',
        groupRatioMultiplier: 0.8,
      })
      expect(summary).not.toBeNull()
      if (!summary) throw new Error('Expected an expandable price projection')
      expect(
        summary.primaryEntries.find((entry) => entry.field === 'constant')
      ).toMatchObject({
        value: 0,
        formatted: '$0',
        formattedRange: '$0 – $0.8',
        unit: 'request',
      })
      if (model.billing_usage_schema) {
        expect(
          summary.primaryEntries.find((entry) => entry.field === 'clips')
        ).toMatchObject({ formattedRange: '$0 – $0.16' })
        expect(
          summary.primaryEntries.find((entry) => entry.field === 'tokens')
        ).toMatchObject({ formattedRange: '$0 – $1.6' })
      } else {
        expect(
          summary.primaryEntries.find(
            (entry) => entry.field === 'imageOutputPrice'
          )
        ).toMatchObject({ formattedRange: '$0 – $0.008' })
      }
      expect(model.billing_display).toEqual(before)
    }
  )

  it.each(conditionalModels)(
    'renders conditional fees on cards and table cells for $billing_expr',
    (model) => {
      const discounted = { ...model, group_ratio: { default: 0.8 } }
      const card = render(
        <ModelCard model={discounted} onClick={vi.fn()} tokenUnit='K' />
      )
      expect(
        within(card.container).getByText('Additional charge')
      ).toBeInTheDocument()
      expect(within(card.container).getByText('$0 – $0.8')).toBeInTheDocument()
      const cell = render(
        <ModelPriceCell model={discounted} options={{ tokenUnit: 'K' }} />
      )
      expect(
        within(cell.container).getByText('Additional charge')
      ).toBeInTheDocument()
      expect(cell.container).toHaveTextContent('0 – 0.8/request')
    }
  )
})

const billingKinds: Array<{
  name: string
  model: PricingModel
  base: number
  fixedCharge?: number
}> = [
  { name: 'token', model: pricingModel(), base: 2 },
  {
    name: 'request',
    model: pricingModel({ quota_type: 1, model_price: 10 }),
    base: 10,
  },
  {
    name: 'dynamic token',
    model: pricingModel({
      billing_mode: 'tiered_expr',
      billing_expr: 'tier("base", p * 2 + 10000) + 20000',
      billing_display: billingDisplayFixture(
        'tier("base", p * 2 + 10000) + 20000'
      ),
    }),
    base: 2,
  },
  {
    name: 'task usage',
    model: pricingModel({
      billing_mode: 'tiered_expr',
      billing_expr: 'tier("music", u("clips") * 0.22)',
      billing_display: taskBillingDisplayFixture(
        'tier("music", u("clips") * 0.22)'
      ),
      billing_usage_schema: { clips: { type: 'number', unit: 'count' } },
    }),
    base: 0.22,
  },
  {
    name: 'conditional native fees',
    model: conditionalModels[0],
    base: 2,
    fixedCharge: 1,
  },
  {
    name: 'conditional task fees',
    model: conditionalModels[1],
    base: 0.4,
    fixedCharge: 1,
  },
]

describe.each(billingKinds)(
  '$name contract group prices',
  ({ model, base, fixedCharge }) => {
    it.each([
      {
        name: 'effective model ratio applied once',
        ratio: 1.6,
        expectedRatio: 1.6,
      },
      { name: 'explicit zero model ratio', ratio: 0, expectedRatio: 0 },
      {
        name: 'global ratio without model override',
        ratio: undefined,
        expectedRatio: 2,
      },
    ])('$name', ({ ratio, expectedRatio }) => {
      const client = new QueryClient({
        defaultOptions: { queries: { retry: false, gcTime: 0 } },
      })
      clients.push(client)
      render(
        <QueryClientProvider client={client}>
          <ModelDetailsContent
            model={{
              ...model,
              // The API has already applied the 0.8 contract discount to the 2x group ratio.
              group_ratio: ratio === undefined ? undefined : { default: ratio },
            }}
            groupRatio={{ default: 2 }}
            usableGroup={{ default: { desc: '', ratio: 2 } }}
            endpointMap={{}}
            autoGroups={[]}
            priceRate={1}
            usdExchangeRate={7}
            tokenUnit='M'
          />
        </QueryClientProvider>
      )
      const section = screen.getByText('Pricing by Group').closest('section')
      if (!section) throw new Error('Expected the group pricing section')
      expect(
        within(section).getAllByText(
          `$${Number((base * expectedRatio).toFixed(6))}`
        ).length
      ).toBeGreaterThan(0)
      expect(section).toHaveTextContent(`${expectedRatio}x`)
      if (fixedCharge !== undefined) {
        expect(section).toHaveTextContent('Additional charge')
        expect(
          within(section).getAllByText(
            `$${fixedCharge * expectedRatio}${model.billing_usage_schema ? '' : '/request'}`
          ).length
        ).toBeGreaterThan(0)
      }
    })
  }
)
