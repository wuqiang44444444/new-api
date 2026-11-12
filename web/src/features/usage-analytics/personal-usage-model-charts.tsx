import { useTranslation } from 'react-i18next'
import { Pie, PieChart } from 'recharts'

import { ReportCard } from '@/components/report-card'
import { formatCustomerStatementQuota } from '@/features/billing-reconciliation/lib'
import { formatNumber, formatPercent } from '@/lib/format'
import { cn } from '@/lib/utils'

import type {
  UsageBillingMode,
  UsageNonTokenMetrics,
  UsageTokenMetrics,
} from './types'

export type UsageModelRank = {
  id: string
  label: string
  billingMode?: UsageBillingMode
  calls: number
  failureCalls?: number
  tokens: number
  missingTokens: boolean
  missingTokenRows?: number
  tokenUsage?: UsageTokenMetrics
  amount?: number
  partialAmount: boolean
  nonToken?: UsageNonTokenMetrics
}

type ChartRow = {
  id: string
  label: string
  value: number
  fill?: string
}

type ChartMetric = 'calls' | 'tokens' | 'amount'

const sliceColors = [
  'var(--chart-1)',
  'var(--chart-2)',
  'var(--chart-3)',
  'var(--chart-4)',
  'var(--chart-5)',
]

export function PersonalUsageModelCharts(props: {
  rows: UsageModelRank[]
  upstream?: boolean
}) {
  const { t } = useTranslation()
  const label = props.upstream
    ? t('Upstream model breakdown')
    : t('Model ranking')
  const colors = new Map(
    props.rows.map((row, index) => [
      row.id,
      sliceColors[index] ?? `hsl(${(index * 137.508) % 360} 65% 52%)`,
    ])
  )
  return (
    <section aria-label={label} className='min-w-0'>
      {!props.upstream && <h2 className='mb-2 font-medium'>{label}</h2>}
      <div role='group' aria-label={label} className='@container'>
        <div className='grid items-start gap-3 @min-[48rem]:grid-cols-3'>
          <ModelMetricChart
            rows={props.rows}
            colors={colors}
            metric='calls'
            title={t('Calls')}
          />
          <ModelMetricChart
            rows={props.rows}
            colors={colors}
            metric='tokens'
            title={t('Total tokens')}
          />
          <ModelMetricChart
            rows={props.rows}
            colors={colors}
            metric='amount'
            title={props.upstream ? t('Reference amount') : t('Net amount')}
            pendingLabel={
              props.upstream ? t('Pending reconciliation') : undefined
            }
          />
        </div>
      </div>
    </section>
  )
}

function ModelMetricChart(props: {
  rows: UsageModelRank[]
  colors: ReadonlyMap<string, string>
  metric: ChartMetric
  title: string
  pendingLabel?: string
}) {
  const { t } = useTranslation()
  const { metric } = props
  const formatValue =
    metric === 'amount' ? formatCustomerStatementQuota : formatNumber
  const partial = props.rows.some((row) =>
    metric === 'tokens'
      ? row.missingTokens
      : metric === 'amount' && row.partialAmount
  )
  const rows = props.rows
    .flatMap((row) => {
      const value = row[metric]
      if (value == null) return []
      if (
        value === 0 &&
        ((metric === 'tokens' && row.missingTokens) ||
          (metric === 'amount' && row.partialAmount && !props.pendingLabel))
      ) {
        return []
      }
      return [
        {
          id: row.id,
          label: row.label === 'unknown' ? t('Unknown model') : row.label,
          value,
        },
      ]
    })
    .sort((a, b) => b.value - a.value || a.label.localeCompare(b.label))

  const total = rows.reduce((sum, row) => sum + row.value, 0)
  const signed = rows.some((row) => row.value < 0)
  const positive = rows.filter((row) => row.value > 0)
  const slices = positive.slice(0, sliceColors.length).map((row) => ({
    ...row,
    fill: props.colors.get(row.id),
  }))
  const other = positive.slice(sliceColors.length)
  const otherTotal = other.reduce((sum, row) => sum + row.value, 0)
  const otherSlice =
    otherTotal > 0
      ? {
          id: '',
          label: t('Other'),
          value: otherTotal,
          fill: 'var(--muted-foreground)',
        }
      : undefined
  const pieData = otherSlice ? [...slices, otherSlice] : slices
  const pending = partial && rows.length === 0 && props.pendingLabel
  const totalLabel = rows.length
    ? formatValue(total)
    : pending || t('unrecorded')

  return (
    <ReportCard
      role='group'
      aria-label={props.title}
      className='gap-1.5 py-1.5'
    >
      <div className='flex flex-wrap items-baseline gap-x-2 gap-y-0.5'>
        <h3 className='text-muted-foreground text-xs font-medium'>
          {props.title}
        </h3>
        <p
          className={cn(
            'text-base/5 font-semibold tracking-tight wrap-anywhere tabular-nums',
            rows.length === 0 && 'text-muted-foreground'
          )}
        >
          {totalLabel}
        </p>
        {partial && rows.length > 0 && (
          <p className='text-muted-foreground text-xs'>{t('Known subtotal')}</p>
        )}
      </div>
      {signed && <SignedModelBars rows={rows} />}
      {!signed && rows.length > 0 && (
        <div className='flex items-start gap-3'>
          {pieData.length > 0 && (
            <div className='shrink-0' aria-hidden='true'>
              <PieChart width={72} height={72} accessibilityLayer={false}>
                <Pie
                  data={pieData}
                  dataKey='value'
                  nameKey='label'
                  innerRadius={24}
                  outerRadius={35}
                  stroke='none'
                  isAnimationActive={false}
                />
              </PieChart>
            </div>
          )}
          <ul className='min-w-0 flex-1 space-y-1 text-xs'>
            {slices.map((row) => (
              <li key={row.id}>
                <ModelLegendRow
                  row={row}
                  value={formatValue(row.value)}
                  share={row.value / total}
                />
              </li>
            ))}
            {otherSlice && (
              <li>
                <details>
                  <summary className='cursor-pointer'>
                    <span className='ml-1 inline-block w-[calc(100%-1.5rem)] align-middle'>
                      <ModelLegendRow
                        row={otherSlice}
                        value={formatValue(otherTotal)}
                        share={otherTotal / total}
                      />
                    </span>
                  </summary>
                  <ul className='mt-1 space-y-1 pl-4'>
                    {other.map((row) => (
                      <li key={row.id}>
                        <ModelLegendRow
                          row={row}
                          value={formatValue(row.value)}
                          share={row.value / total}
                        />
                      </li>
                    ))}
                  </ul>
                </details>
              </li>
            )}
            {rows
              .filter((row) => row.value === 0)
              .map((row) => (
                <li key={row.id}>
                  <ModelLegendRow row={row} value={formatValue(0)} />
                </li>
              ))}
          </ul>
        </div>
      )}
      {rows.length === 0 && !pending && (
        <p className='text-muted-foreground text-xs'>
          {t('No data available')}
        </p>
      )}
    </ReportCard>
  )
}

function ModelLegendRow(props: {
  row: ChartRow
  value: string
  share?: number
}) {
  return (
    <span className='flex items-start gap-2'>
      <span
        aria-hidden='true'
        className='mt-1 size-2 shrink-0 rounded-full'
        style={{ backgroundColor: props.row.fill ?? 'var(--muted-foreground)' }}
      />
      <span className='min-w-0 flex-1 wrap-anywhere'>{props.row.label}</span>
      <span className='max-w-[60%] text-right font-medium wrap-anywhere tabular-nums'>
        {props.value}
        {props.share !== undefined && (
          <span className='text-muted-foreground ml-1 font-normal'>
            {formatPercent(props.share * 100)}
          </span>
        )}
      </span>
    </span>
  )
}

function SignedModelBars(props: { rows: ChartRow[] }) {
  const max = props.rows.reduce(
    (value, row) => Math.max(value, Math.abs(row.value)),
    0
  )
  return (
    <ul className='space-y-2 text-xs'>
      {props.rows.map((row) => {
        const negative = row.value < 0
        const color = negative ? 'var(--destructive)' : sliceColors[0]
        return (
          <li key={row.id}>
            <ModelLegendRow
              row={{ ...row, fill: color }}
              value={formatCustomerStatementQuota(row.value)}
            />
            <div
              aria-hidden='true'
              className='bg-muted relative mt-1 h-2 rounded'
            >
              <span className='bg-border absolute inset-y-0 left-1/2 w-px' />
              <span
                className='absolute inset-y-0 rounded'
                style={{
                  backgroundColor: color,
                  width: `${(Math.abs(row.value) / max) * 50}%`,
                  ...(negative ? { right: '50%' } : { left: '50%' }),
                }}
              />
            </div>
          </li>
        )
      })}
    </ul>
  )
}
