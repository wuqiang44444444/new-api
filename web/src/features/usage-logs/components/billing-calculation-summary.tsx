import { useTranslation } from 'react-i18next'

import {
  recordedExpressionRows,
  type BillingCalculation,
} from '../lib/billing-calculation'

// Display rounding only. Saved operands/results are never used to recalculate
// a charge; the original records remain available with their full precision.
function compactCalculationNumber(value: string): string {
  const number = Number(value)
  if (!Number.isFinite(number) || !value.trim() || Math.abs(number) >= 1e15) {
    return value
  }
  if (number !== 0 && Math.abs(number) < 0.000001) {
    return Number(number.toPrecision(6)).toString()
  }
  return Number(number.toFixed(6)).toString()
}

function RecordedEquation(props: { formula: string; result: string }) {
  const formula = props.formula.replaceAll(
    /\b\d+(?:\.\d+)?(?:e[+-]?\d+)?\b/gi,
    compactCalculationNumber
  )
  const result = compactCalculationNumber(props.result)
  const approximate = formula !== props.formula || result !== props.result
  return (
    <p className='break-words tabular-nums'>
      {formula} {approximate ? '≈' : '='} {result}
    </p>
  )
}

export function BillingCalculationSummary(props: {
  calculation: BillingCalculation
}) {
  const { t } = useTranslation()
  const calculation = props.calculation
  const nodes = new Map(calculation.nodes?.map((node) => [node.id, node]))
  const rows = recordedExpressionRows(calculation, t)
  const usageLabels: Record<string, string> = {
    'usage:duration_seconds': t('Billable duration (seconds)'),
    'usage:seconds': t('Billable duration (seconds)'),
    'usage:tokens': t('Billable tokens'),
    p: t('Normalized input tokens'),
    c: t('Normalized output tokens'),
    usd_exchange_rate: t('Exchange rate (CNY per USD)'),
  }
  const facts = rows.filter(
    (row) => usageLabels[row.op] && row.rawResult !== 'protected'
  )
  const computations = rows.filter(
    (row) =>
      !usageLabels[row.op] &&
      !row.op.startsWith('usage:') &&
      ![
        'tier',
        'if',
        '_trace',
        '_trace_int',
        'param',
        'header',
        'protected_request',
        'variable',
      ].includes(row.op) &&
      row.rawResult.trim() !== '' &&
      Number.isFinite(Number(row.rawResult))
  )
  const currencyConversions = new Set<number>()
  const cnyCharges = new Set<number>()
  for (const row of computations) {
    const node = nodes.get(row.id)
    if (
      node?.op === '/' &&
      nodes.get(node.args?.[1] ?? -1)?.op === 'usd_exchange_rate'
    ) {
      currencyConversions.add(row.id)
      if (node.args?.[0] !== undefined) cnyCharges.add(node.args[0])
    }
  }
  const stepLabels: Record<string, string> = {
    quota_conversion: t('Convert to account quota'),
    group_ratio: t('Apply group ratio'),
    contract_ratio: t('Apply contract discount'),
    round: t('Round to nearest integer'),
    truncate: t('Truncate to integer'),
    ceil: t('Round up'),
    refund: t('Full refund'),
    per_call: t('Per-call charge'),
    other_ratios: t('Apply additional ratios'),
    minimum_charge: t('Minimum charge'),
  }
  return (
    <div className='space-y-4'>
      {(facts.length > 0 || calculation.matched_tier) && (
        <dl className='bg-muted/40 grid gap-2 rounded-md p-3 text-sm sm:grid-cols-2'>
          {calculation.matched_tier && (
            <div>
              <dt className='text-muted-foreground'>{t('Matched Tier')}</dt>
              <dd className='font-medium break-words'>
                {calculation.matched_tier}
              </dd>
            </div>
          )}
          {facts.map((row, index) => (
            // Evaluation occurrences retain their original order.
            // eslint-disable-next-line react/no-array-index-key
            <div key={`${row.id}-${index}`}>
              <dt className='text-muted-foreground'>{usageLabels[row.op]}</dt>
              <dd className='font-medium tabular-nums'>
                {compactCalculationNumber(row.rawResult)}
              </dd>
            </div>
          ))}
        </dl>
      )}
      <ol className='space-y-3 text-sm'>
        {computations.map((row, index) => {
          let label = t('Price calculation')
          if (currencyConversions.has(row.id)) label = t('Charge in USD')
          else if (cnyCharges.has(row.id)) label = t('Base charge (CNY)')
          return (
            // eslint-disable-next-line react/no-array-index-key
            <li key={`${row.id}-${index}`}>
              <p className='text-muted-foreground'>{label}</p>
              <RecordedEquation formula={row.formula} result={row.rawResult} />
            </li>
          )
        })}
        {calculation.steps?.map((step, index) => (
          // eslint-disable-next-line react/no-array-index-key
          <li key={`host-${index}`}>
            <p className='text-muted-foreground'>
              {stepLabels[step.op] ?? t('Price calculation')}
            </p>
            <RecordedEquation formula={step.formula} result={step.result} />
          </li>
        ))}
      </ol>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Rounded values are marked with ≈. Expand the original records for full precision and condition results.'
        )}
      </p>
    </div>
  )
}
