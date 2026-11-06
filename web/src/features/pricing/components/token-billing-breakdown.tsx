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
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table'
import { Badge } from '@/components/ui/badge'

import { billingConditionText } from '../lib/billing-condition-text'
import {
  isUsableBillingDisplay,
  recordedBillingScenario,
} from '../lib/billing-display'
import { BILLING_VARS, type RequestRuleTrace } from '../lib/billing-expr'
import {
  formatTaskUsageUnitPrice,
  type DynamicPriceOptions,
} from '../lib/dynamic-price'
import type { BillingDisplayProjection, BillingDisplayTier } from '../types'
import { imageTokenPriceLabel } from '../lib/image-token-price-display'

type Props = {
  imageGeneration?: boolean
  expression: string
  projection?: BillingDisplayProjection | null
  multiplier: number
  compact: boolean
  matchedTier?: string | null
  traces?: RequestRuleTrace[] | null
  priceOptions?: Pick<
    DynamicPriceOptions,
    'showRechargePrice' | 'priceRate' | 'usdExchangeRate'
  >
}

type PriceRow = { tier: BillingDisplayTier; matched?: boolean }

/** Token prices keep the full projection, including conditions and fixed fees. */
export function TokenBillingBreakdown(props: Props) {
  const { t, i18n } = useTranslation()
  const projection = props.projection
  if (!isUsableBillingDisplay(projection)) {
    return (
      <section className='space-y-2 py-3'>
        <p className='text-muted-foreground text-sm'>
          {t('Unable to parse structured pricing')}
        </p>
        <p className='text-xs'>{t('Raw expression')}</p>
        <code className='block text-xs break-all'>{props.expression}</code>
      </section>
    )
  }
  const amount = (value: number) =>
    formatTaskUsageUnitPrice(value, {
      tokenUnit: 'M',
      imageGeneration: props.imageGeneration,
      ...props.priceOptions,
      groupRatioMultiplier: props.multiplier,
    })
  const recordedScenario = recordedBillingScenario(projection, props.traces)
  const rule = projection.rules?.length === 1 ? projection.rules[0] : undefined
  const rows: PriceRow[] =
    projection.scenarios?.length && rule
      ? projection.scenarios.flatMap((scenario) =>
          scenario.tiers.map((tier) => ({ tier, matched: scenario.matched }))
        )
      : (projection.tiers ?? []).map((tier) => ({ tier }))
  const variables = [
    ...new Set(rows.flatMap(({ tier }) => Object.keys(tier.unit_prices))),
  ]
    .sort(
      (a, b) =>
        BILLING_VARS.findIndex((item) => item.key === a) -
        BILLING_VARS.findIndex((item) => item.key === b)
    )
  const labelPrices = {
    imagePrice: rows.every(({ tier }) => typeof tier.unit_prices.img === 'number') ? rows[0]?.tier.unit_prices.img : undefined,
    imageCachePrice: rows.every(({ tier }) => typeof tier.unit_prices.img_cr === 'number') ? rows[0]?.tier.unit_prices.img_cr : undefined,
    imageOutputPrice: rows.find(({ tier }) => typeof tier.unit_prices.img_o === 'number')?.tier.unit_prices.img_o,
  }
  const hasFixed =
    rows.some(({ tier }) => tier.has_constant) ||
    projection.constant_charge !== undefined
  const occurrences = new Map<string, number>()
  return (
    <section className='space-y-3 py-3'>
      <div className='space-y-1'>
        {!props.compact && (
          <p className='font-medium'>{t('Dynamic Pricing')}</p>
        )}
        <p className='text-muted-foreground text-xs'>
          {t('Prices per 1M tokens; fixed charges are per request.')}
        </p>
      </div>
      <StaticDataTable
        data={rows}
        getRowKey={(_, index) => String(index)}
        columns={[
          {
            id: 'condition',
            header: t('Applicable conditions'),
            cellClassName: 'min-w-56 max-w-lg whitespace-normal break-words',
            cell: ({ tier, matched }) => {
              // A tier label alone never proves that a time multiplier fired.
              const hit =
                props.matchedTier === tier.label &&
                (matched === undefined || recordedScenario?.matched === matched)
              return (
                <div className='space-y-1.5'>
                  <div className='flex flex-wrap gap-1.5'>
                    <Badge variant='secondary'>{tier.label}</Badge>
                    {matched !== undefined && rule && (
                      <Badge variant='outline'>
                        {matched ? t('Condition met') : t('Otherwise')} ×
                        {matched ? rule.multiplier : (rule.fallback ?? 1)}
                      </Badge>
                    )}
                    {hit && <Badge variant='outline'>{t('Matched')}</Badge>}
                  </div>
                  {matched !== undefined && rule && (
                    <p className='text-xs'>
                      {matched
                        ? billingConditionText(rule, t, i18n.language)
                        : `${t('Not')} (${billingConditionText(rule, t, i18n.language)})`}
                    </p>
                  )}
                  {(tier.condition || tier.condition_text) && (
                    <p className='text-xs'>
                      {tier.condition
                        ? billingConditionText(tier.condition, t, i18n.language)
                        : tier.condition_text}
                    </p>
                  )}
                </div>
              )
            },
          },
          ...variables.map((variable) => ({
            id: variable,
            header: t(
              imageTokenPriceLabel(variable, BILLING_VARS.find((item) => item.key === variable)?.shortLabel ?? variable, props.imageGeneration, labelPrices)
            ),
            cellClassName: 'text-right font-mono tabular-nums',
            cell: ({ tier }: PriceRow) => (
              <span title={props.imageGeneration ? t('Unit price before discounts: {{price}} USD per 1M tokens.', { price: tier.unit_prices[variable] ?? 0 }) : undefined}>
                {amount(tier.unit_prices[variable] ?? 0)}
              </span>
            ),
          })),
          ...(hasFixed
            ? [
                {
                  id: 'constant',
                  header: t('Additional charge'),
                  cellClassName: 'text-right font-mono tabular-nums',
                  cell: ({ tier }: PriceRow) =>
                    `${amount((tier.constant ?? 0) + (projection.constant_charge ?? 0))}/${t('request')}`,
                },
              ]
            : []),
        ]}
      />
      {!projection.scenarios?.length && !!projection.rules?.length && (
        <div className='space-y-1'>
          <p className='text-sm font-medium'>{t('Conditional multipliers')}</p>
          {projection.rules.map((item) => {
            const identity = JSON.stringify(item)
            const occurrence = occurrences.get(identity) ?? 0
            occurrences.set(identity, occurrence + 1)
            return (
              <p key={`${identity}:${occurrence}`} className='text-xs'>
                {billingConditionText(item, t, i18n.language)}: ×
                {item.multiplier}; {t('Otherwise')}: ×{item.fallback ?? 1}
              </p>
            )
          })}
        </div>
      )}
      {props.matchedTier && !!projection.rules?.length && !recordedScenario && (
        <p className='text-muted-foreground text-xs'>
          {t('Historical condition results are unavailable.')}
        </p>
      )}
    </section>
  )
}
