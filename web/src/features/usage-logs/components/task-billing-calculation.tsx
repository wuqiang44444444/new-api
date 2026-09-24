import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { api } from '@/lib/api'
import { formatLogQuota } from '@/lib/format'

import {
  recordedExpressionRows,
  type BillingCalculation,
  type TaskCalculation,
} from '../lib/billing-calculation'
import { BillingCalculationSummary } from './billing-calculation-summary'

function CalculationSteps(props: { calculation: BillingCalculation }) {
  const { t } = useTranslation()
  const rows = recordedExpressionRows(props.calculation, t)
  const labels: Record<string, string> = {
    group_ratio: t('Group ratio'),
    contract_ratio: t('Contract discount'),
    round: t('Round to nearest integer'),
    truncate: t('Truncate to integer'),
    ceil: t('Round up'),
    minimum_charge: t('Minimum charge'),
    default_if_zero: t('Default used for missing or zero usage'),
    refund: t('Full refund'),
    free: t('No charge'),
    not_charged: t('No charge'),
    no_billable_usage: t('No charge'),
    input_tokens: t('Input Tokens'),
    output_tokens: t('Output Tokens'),
    cache_tokens: t('Cached Tokens'),
    violation_fee: t('Failure fee'),
    budget_range: t('Estimated range'),
    budget: t('Estimated upper bound'),
  }
  const units: Record<string, string> = { quota: t('Quota'), token: 'Token' }
  return (
    <div className='min-w-0 space-y-2'>
      <ol className='min-w-0 list-inside list-decimal space-y-1 text-xs'>
        {rows.map((row, index) => (
          <li // Recorded rows cannot be reordered; occurrence position is their identity.
            // eslint-disable-next-line react/no-array-index-key
            key={`${row.id}-${index}`}
            className='break-words'
          >
            <span className='font-mono'>
              #{row.id}: {row.formula} = {row.result}
            </span>
          </li>
        ))}
        {props.calculation.steps?.map((step, index) => (
          <li // Recorded host steps are immutable and ordered.
            // eslint-disable-next-line react/no-array-index-key
            key={`step-${index}`}
            className='break-words'
          >
            {labels[step.op] && <span>{labels[step.op]}: </span>}
            <span className='font-mono'>
              {step.formula} = {step.result}
            </span>{' '}
            {units[step.unit ?? ''] ?? ''}
          </li>
        ))}
      </ol>
      <p className='font-medium'>
        {t('Calculated quota')}: {props.calculation.quota}
      </p>
    </div>
  )
}

export function BillingCalculationView(props: { data: TaskCalculation }) {
  const { t } = useTranslation()
  const data = props.data
  const pending = data.state === 'pending' || data.state === 'awaiting_usage'
  const states: Record<string, string> = {
    pending: t('Awaiting final settlement'),
    settled: t('Settled'),
    refunded: t('Refunded'),
    failed: t('Settlement failed'),
    debt: t('Insufficient balance'),
    awaiting_usage: t('Awaiting usage'),
  }
  const missing = (status: string) =>
    status === 'historical'
      ? t(
          'Billing details were not fully recorded. The calculation cannot be reconstructed.'
        )
      : t(
          'The billing calculation is missing or inconsistent. Please contact support.'
        )
  const current = data.source === 'initial' ? data.initial : data.settlement
  return (
    <section
      className='min-w-0 space-y-3 rounded-md border p-3 text-sm'
      aria-label={t('Charge calculation')}
    >
      <h3 className='font-medium'>{t('Charge calculation')}</h3>
      <p>
        {t('Settlement')}: {states[data.state] ?? t('Unknown')}
      </p>
      <div className='bg-muted/40 space-y-1 rounded-md p-3'>
        <p className='text-muted-foreground'>
          {pending ? t('Precharged amount') : t('Charged quota')}
        </p>
        {data.charge_unknown ? (
          <p>{t('Unknown')}</p>
        ) : (
          <>
            <p className='text-2xl font-semibold tabular-nums'>
              {formatLogQuota(data.quota)}
            </p>
            <p className='text-muted-foreground text-xs tabular-nums'>
              {data.quota} {t('Quota')}
            </p>
          </>
        )}
        {pending && (
          <p className='text-muted-foreground text-sm'>
            {t(
              'The amount has been reserved. The final charge will be confirmed when the task finishes.'
            )}
          </p>
        )}
      </div>
      {data.target_quota !== undefined && data.state !== 'settled' && (
        <p>
          {t('Pending settlement quota')}: {data.target_quota}
        </p>
      )}
      {data.source === 'not_charged' && (
        <p>
          {t(
            'Complete batch results confirm no charge for this request. Any precharge is released when settlement completes.'
          )}
        </p>
      )}
      {data.source === 'initial' && data.initial && !pending && (
        <p className='text-muted-foreground'>
          {t(
            'Uses the recorded precharge calculation. Estimated usage is not measured usage.'
          )}
        </p>
      )}
      {data.evidence !== 'complete' && (
        <p role='status'>{missing(data.evidence)}</p>
      )}
      {current && (
        <>
          <BillingCalculationSummary calculation={current} />
          <Collapsible>
            <CollapsibleTrigger className='text-primary min-h-9 text-sm underline underline-offset-4'>
              {t('Original calculation records')}
            </CollapsibleTrigger>
            <CollapsibleContent className='space-y-3 pt-2'>
              <CalculationSteps calculation={current} />
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Protected request values are hidden. Condition results and monetary operands are retained.'
                )}
              </p>
            </CollapsibleContent>
          </Collapsible>
        </>
      )}
      {data.source !== 'initial' && (
        <Collapsible>
          <CollapsibleTrigger className='text-primary text-sm underline underline-offset-4'>
            {t('Precharge calculation')}
          </CollapsibleTrigger>
          <CollapsibleContent className='pt-2'>
            <p className='text-muted-foreground mb-2'>{t('Estimated usage')}</p>
            {data.initial ? (
              <CalculationSteps calculation={data.initial} />
            ) : (
              <p>{missing(data.initial_evidence)}</p>
            )}
          </CollapsibleContent>
        </Collapsible>
      )}
      {data.refunded_quota !== undefined && (
        <p className='font-mono text-xs'>
          {t('Refunded quota')}: {data.refunded_quota}; {t('Net charge')}:{' '}
          {data.refunded_quota} − {data.refunded_quota} = {data.quota}{' '}
          {t('Quota')}
        </p>
      )}
      {!!data.waived_quota && (
        <p>
          {t('Waived unpaid quota')}: {data.waived_quota}
        </p>
      )}
      {data.initial && data.state === 'settled' && (
        <p className='font-mono text-xs'>
          {t('Net charge')}: {data.initial.quota} + (
          {data.quota - data.initial.quota}) = {data.quota} {t('Quota')}
        </p>
      )}
      <p className='text-muted-foreground text-xs'>
        {t(
          'The displayed currency uses current display settings. Calculation steps retain the original quota values.'
        )}
      </p>
    </section>
  )
}

export function TaskBillingCalculation(props: {
  taskId: string
  batchLine?: string
}) {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: ['task-billing-calculation', props.taskId, props.batchLine],
    queryFn: async () => {
      const url =
        props.batchLine === undefined
          ? `/api/task/${encodeURIComponent(props.taskId)}/billing`
          : `/api/batch/${encodeURIComponent(props.taskId)}/billing/calculation`
      const response = await api.get<{
        success: boolean
        data: TaskCalculation
      }>(url, {
        params:
          props.batchLine === undefined
            ? undefined
            : { custom_id: props.batchLine },
      })
      if (!response.data.success) {
        throw new Error('Billing calculation unavailable')
      }
      return response.data.data
    },
  })
  if (query.isPending) return <LoadingState size='sm' className='min-h-20' />
  if (query.isError) {
    return (
      <ErrorState
        title={t('Loading failed')}
        onRetry={() => void query.refetch()}
        className='min-h-20'
      />
    )
  }
  return <BillingCalculationView data={query.data} />
}
