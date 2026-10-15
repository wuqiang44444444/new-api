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
import { useId, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  formatPricingAmount,
  type PricingCurrency,
} from '@/features/model-pricing/currency'
import { BILLING_EXTRA_VARS } from '@/features/pricing/lib/billing-expr'
import {
  buildEstimatorTokens,
  type ExtraTokenValues,
} from '@/features/pricing/lib/tier-expr'
import { cn } from '@/lib/utils'

import { DraftNumberInput } from './draft-number-input'
import { RequestSimulation } from './request-simulation'
import { useBillingExpressionPreview } from './use-billing-expression-preview'

// ---------------------------------------------------------------------------
// Cost estimator
// ---------------------------------------------------------------------------

type EstimatorProps = {
  currency: PricingCurrency
  effectiveExpr: string
  fullExpr: string
}

export function TokenExpressionPreview({
  effectiveExpr,
  fullExpr,
  currency,
}: EstimatorProps) {
  const { t } = useTranslation()
  const inputId = useId()
  const outputId = useId()
  const lengthId = useId()
  const [lengthOverride, setLengthOverride] = useState('')
  const [promptTokens, setPromptTokens] = useState(0)
  const [completionTokens, setCompletionTokens] = useState(0)
  const [extras, setExtras] = useState<ExtraTokenValues>({
    cacheReadTokens: 0,
    cacheCreateTokens: 0,
    cacheCreate1hTokens: 0,
    imageTokens: 0,
    imageCacheTokens: 0,
    imageOutputTokens: 0,
    audioInputTokens: 0,
    audioOutputTokens: 0,
  })

  const tokens = useMemo(() => {
    const values = buildEstimatorTokens(promptTokens, completionTokens, extras)
    if (lengthOverride.trim()) values.len = Number(lengthOverride)
    return values
  }, [promptTokens, completionTokens, extras, lengthOverride])

  const preview = useBillingExpressionPreview(
    effectiveExpr.trim()
      ? {
          key: 'preview',
          expression: effectiveExpr,
          sample: { usage_semantic: 'normalized', normalized_usage: tokens },
        }
      : null
  )
  const result = preview.result?.evaluation
  const error = preview.result?.error

  return (
    <div className='bg-muted/30 space-y-3 rounded-md border p-3'>
      <div className='space-y-1'>
        <h4 className='text-sm font-medium'>{t('Token estimator')}</h4>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Enter token counts to preview the estimated cost (excluding group multipliers).'
          )}
        </p>
      </div>
      <div className='grid grid-cols-2 gap-3'>
        <div className='space-y-1'>
          <Label htmlFor={inputId} className='text-xs'>
            {t('Input tokens')}
          </Label>
          <DraftNumberInput
            id={inputId}
            min={0}
            value={promptTokens}
            onValueChange={setPromptTokens}
          />
        </div>
        <div className='space-y-1'>
          <Label htmlFor={outputId} className='text-xs'>
            {t('Output tokens')}
          </Label>
          <DraftNumberInput
            id={outputId}
            min={0}
            value={completionTokens}
            onValueChange={setCompletionTokens}
          />
        </div>
      </div>
      <Field>
        <FieldLabel htmlFor={lengthId}>
          {t('Full input length override')}
        </FieldLabel>
        <Input
          id={lengthId}
          type='number'
          min={0}
          value={lengthOverride}
          onChange={(event) => setLengthOverride(event.target.value)}
          placeholder={t('Use the existing token total')}
        />
      </Field>
      <div className='grid grid-cols-2 gap-3'>
        {BILLING_EXTRA_VARS.map((variable) => {
          // BILLING_EXTRA_VARS only contains pricing variables; they are
          // guaranteed to have a non-null `field` (the `len` condition-only
          // variable is filtered out). Narrow the type here for safety.
          if (!variable.field) return null
          const stateKey = variable.field.replace(
            'Price',
            'Tokens'
          ) as keyof ExtraTokenValues
          return (
            <div key={variable.key} className='space-y-1'>
              <Label htmlFor={`${inputId}-${variable.key}`} className='text-xs'>
                {t(variable.shortLabel)}
              </Label>
              <DraftNumberInput
                id={`${inputId}-${variable.key}`}
                min={0}
                value={extras[stateKey]}
                onValueChange={(value) =>
                  setExtras((prev) => ({
                    ...prev,
                    [stateKey]: value,
                  }))
                }
              />
            </div>
          )
        })}
      </div>
      <div
        role={error ? 'alert' : 'status'}
        aria-busy={preview.pending}
        className={cn(
          'rounded-md border p-3 text-sm',
          error
            ? 'border-destructive/50 bg-destructive/10 text-destructive'
            : 'border-primary/50 bg-primary/10'
        )}
      >
        {error && (
          <span>
            {t('Expression error')}: {t(error)}
          </span>
        )}
        {preview.pending && <span>{t('Calculating...')}</span>}
        {result && !error && (
          <div className='flex flex-wrap items-center gap-2'>
            <span className='font-medium'>
              {t('Estimated cost')}:{' '}
              {formatPricingAmount(result.raw_cost_usd, currency)}
              {result.billing_unit === 'request' && `/${t('request')}`}
            </span>
            {result.usd_exchange_rate && (
              <span className='text-muted-foreground text-xs'>
                {t('Applied exchange rate (CNY/USD)')}:{' '}
                {result.usd_exchange_rate.rate}
              </span>
            )}
            {result.saturated && (
              <span>{t('The sample exceeds the quota limit.')}</span>
            )}
            {result.matched_tier && (
              <Badge variant='outline' className='text-xs'>
                {t('Hit tier')}: {result.matched_tier}
              </Badge>
            )}
          </div>
        )}
      </div>
      <RequestSimulation
        expression={fullExpr}
        tokens={tokens}
        currency={currency}
        mode='token'
      />
    </div>
  )
}
