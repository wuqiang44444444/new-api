import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, expect, it } from 'vitest'

import { getPricingQueryKey } from '@/features/pricing/hooks/use-pricing-data'
import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { usageLogSchema } from '../../data/schema'
import { DetailsDialog } from '../dialogs/details-dialog'

afterEach(cleanup)

it.each(['en', 'zh'])(
  'uses the same image category names for saved lines and prices in %s',
  async (lng) => {
    const i18n = createInstance()
    await i18n.init({
      lng,
      resources: { en, zh },
      interpolation: { escapeValue: false },
    })
    useSystemConfigStore
      .getState()
      .setConfig({
        currency: { ...DEFAULT_CURRENCY_CONFIG, quotaDisplayType: 'USD' },
      })
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity } },
    })
    queryClient.setQueryData(getPricingQueryKey(undefined), {
      data: [],
      vendors: [],
    })
    const log = usageLogSchema.parse({
      id: 1,
      user_id: 1,
      created_at: 1000,
      type: 2,
      content: '',
      model_name: 'image',
      quota: 20770,
      other: JSON.stringify({
        billing_mode: 'tiered_expr',
        request_path: '/v1/images/generations',
        group_ratio: 1,
        contract_applicable: false,
        billing_explanation: {
          lines: [
            {
              label: 'Text Input',
              quantity: 76,
              unit: 'token',
              unit_price_usd: 5,
              subtotal_usd: 0.00038,
            },
            {
              label: 'Image In',
              quantity: 0,
              unit: 'token',
              unit_price_usd: 8,
              subtotal_usd: 0,
            },
            {
              label: 'Image Cache',
              quantity: 0,
              unit: 'token',
              unit_price_usd: 2,
              subtotal_usd: 0,
            },
            {
              label: 'Image Out',
              quantity: 1372,
              unit: 'token',
              unit_price_usd: 30,
              subtotal_usd: 0.04116,
            },
            {
              label: 'Cache Read',
              quantity: 0,
              unit: 'token',
              unit_price_usd: 1.25,
              subtotal_usd: 0,
            },
          ],
        },
      }),
    })
    render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={queryClient}>
          <DetailsDialog
            log={log}
            open
            isAdmin={false}
            isRoot={false}
            onOpenChange={() => {}}
          />
        </QueryClientProvider>
      </I18nextProvider>
    )
    for (const label of lng === 'zh'
      ? ['图片输入', '图片输出', '图片缓存输入']
      : ['Image Input', 'Image Output', 'Cached Image Input']) {
      expect(screen.getByText(label)).toBeVisible()
    }
    expect(
      screen.getByText(lng === 'zh' ? '缓存读取' : 'Cache Read')
    ).toBeVisible()
    expect(
      screen.getByText(lng === 'zh' ? '文本输入单价' : 'Text input price')
        .nextElementSibling
    ).toHaveTextContent('$5/M')
    expect(
      screen.getByText(
        lng === 'zh'
          ? '最终费用按最小计费单位取整。'
          : 'Final charges are rounded to the smallest billing unit.'
      )
    ).toBeVisible()
    queryClient.clear()
  }
)
