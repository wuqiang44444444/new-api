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
import { useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Field, FieldLabel } from '@/components/ui/field'
import { Textarea } from '@/components/ui/textarea'
import {
  formatPricingAmount,
  type PricingCurrency,
} from '@/features/model-pricing/currency'
import {
  previewBillingExpressions,
  type BillingExprPreviewItemResult,
} from '@/features/pricing/lib/billing-display-preview'
import type { BillingUsageSchema } from '@/features/pricing/types'

import { BillingPreviewContext } from './billing-preview-context'
import { parseBillingPreviewContext } from './billing-preview-input'

// Custom task prices use the same server engine and explicit USD unit as
// settlement. No browser parser is allowed to approximate their cost.
export function TaskExpressionPreview(props: {
  expression: string
  schema: BillingUsageSchema
  currency?: PricingCurrency
}) {
  const { t } = useTranslation()
  const usageId = useId()
  const [usageJSON, setUsageJSON] = useState(() =>
    JSON.stringify(
      Object.fromEntries(
        Object.entries(props.schema).map(([name, field]) => {
          if (field.enum) return [name, field.enum[0]]
          if (field.type === 'boolean') return [name, false]
          if (field.unit === 'token') return [name, 1000000]
          return [name, field.unit === 'second' ? 5 : 1]
        })
      ),
      null,
      2
    )
  )
  const [time, setTime] = useState('')
  const [contextJSON, setContextJSON] = useState('')
  const [allowMissing, setAllowMissing] = useState(false)
  const [pending, setPending] = useState(false)
  const [outcome, setOutcome] = useState<{
    key: string
    legacy: boolean
    result: BillingExprPreviewItemResult
  } | null>(null)
  const sequence = useRef(0)
  const key = JSON.stringify([
    props.expression,
    usageJSON,
    time,
    contextJSON,
    allowMissing,
  ])
  const needsContext = /\b(param|header)\s*\(/.test(props.expression)
  const result = outcome?.key === key ? outcome.result : null

  const calculate = async (legacy = false) => {
    const current = ++sequence.current
    setPending(true)
    setOutcome(null)
    try {
      const usage: unknown = JSON.parse(usageJSON)
      if (
        !usage ||
        typeof usage !== 'object' ||
        Array.isArray(usage) ||
        !Object.values(usage).every((value) =>
          ['number', 'string', 'boolean'].includes(typeof value)
        )
      ) {
        throw new Error(
          'Task usage samples must contain numbers, strings or booleans.'
        )
      }
      const context = parseBillingPreviewContext(time, contextJSON)
      if (context.error) throw new Error(context.error)
      if (needsContext && !contextJSON.trim() && !allowMissing) {
        throw new Error(
          'Provide trial context or explicitly simulate missing values.'
        )
      }
      let completionTokens: number | undefined
      if (legacy) {
        const value = (usage as Record<string, unknown>).tokens
        if (
          typeof value !== 'number' ||
          !Number.isFinite(value) ||
          value < 0 ||
          value > 2147483647
        ) {
          throw new Error(
            'Task usage samples must be finite and between 0 and 2147483647.'
          )
        }
        completionTokens = value
      }
      const results = await previewBillingExpressions([
        {
          key: 'task-preview',
          expression: props.expression,
          task_usage: !legacy,
          sample: {
            usage: usage as Record<string, number | string | boolean>,
            completion_tokens: completionTokens,
            body: context.body,
            headers: context.headers,
            pricing_time: context.pricingTime,
          },
        },
      ])
      if (current === sequence.current) {
        setOutcome({
          key,
          legacy,
          result: results[0] ?? {
            key: 'task-preview',
            error: 'Preview failed',
          },
        })
      }
    } catch (error) {
      let message = 'Preview failed'
      if (error instanceof SyntaxError) {
        message =
          'Task usage samples must contain numbers, strings or booleans.'
      } else if (error instanceof Error) {
        message = error.message
      }
      if (current === sequence.current) {
        setOutcome({
          key,
          legacy,
          result: {
            key: 'task-preview',
            error: message,
          },
        })
      }
    } finally {
      if (current === sequence.current) setPending(false)
    }
  }

  return (
    <div className='bg-muted/30 space-y-3 rounded-md border p-3'>
      <h4 className='text-sm font-medium'>{t('Cost calculator')}</h4>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Uses the full expression. Group and contract discounts are not included.'
        )}
      </p>
      <Field>
        <FieldLabel htmlFor={usageId}>{t('Usage parameters')}</FieldLabel>
        <Textarea
          id={usageId}
          value={usageJSON}
          onChange={(event) => setUsageJSON(event.target.value)}
          className='font-mono text-xs'
          rows={6}
        />
      </Field>
      <BillingPreviewContext
        time={time}
        onTimeChange={setTime}
        needsContext={needsContext}
        json={contextJSON}
        onJSONChange={setContextJSON}
        allowMissing={allowMissing}
        onAllowMissingChange={setAllowMissing}
      />
      <Button
        type='button'
        variant='outline'
        disabled={pending || !props.expression.trim()}
        onClick={() => void calculate()}
      >
        {t('Calculate')}
      </Button>
      {result?.error && (
        <p role='alert' className='text-destructive text-xs'>
          {t(result.error)}
        </p>
      )}
      {result?.error ===
        'Token variables cannot be evaluated as task USD pricing.' && (
        <Button
          type='button'
          variant='outline'
          disabled={pending}
          onClick={() => void calculate(true)}
        >
          {t('Preview as legacy token pricing')}
        </Button>
      )}
      {result?.evaluation && (
        <div role='status' className='space-y-1 text-sm'>
          <p>
            {formatPricingAmount(
              result.evaluation.raw_cost_usd,
              props.currency
            )}
          </p>
          {outcome?.legacy && (
            <p className='text-muted-foreground text-xs'>
              {t(
                'Legacy token preview uses tokens as completion usage. Pricing settings are not changed.'
              )}
            </p>
          )}
          <p className='text-muted-foreground text-xs'>
            {t('Matched tier')}: {result.evaluation.matched_tier || '—'}
          </p>
          {result.evaluation.saturated && (
            <p>{t('The sample exceeds the quota limit.')}</p>
          )}
        </div>
      )}
    </div>
  )
}
