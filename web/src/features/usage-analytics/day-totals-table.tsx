import { useTranslation } from 'react-i18next'

import { formatCustomerStatementQuota } from '@/features/billing-reconciliation/lib'

import {
  UsageCallsCell,
  UsageMoneyCell,
  UsageNonTokenCell,
  UsageReferenceAmount,
  UsageQualityCell,
  UsageTokensCell,
} from './metrics-cells'
import type { UsageAnalyticsMetrics } from './types'

export function DayTotalsTable(props: {
  upstream?: boolean
  caption?: string
  dates: string[]
  futureFlags: boolean[]
  unpublishedFlags?: boolean[]
  incompleteDays?: string[] | null
  onDaySelect?: (day: string) => void
  days: UsageAnalyticsMetrics[]
  total: UsageAnalyticsMetrics
}) {
  const { t } = useTranslation()
  // 单日模式下合计列与唯一日期列完全重复，按展示约定隐藏；周视图保留。
  const singleDay = props.dates.length === 1
  return (
    <div className='overflow-x-auto'>
      <table
        className={
          singleDay
            ? 'w-full min-w-[640px] text-sm'
            : 'w-full min-w-[1120px] text-sm'
        }
      >
        {props.caption && (
          <caption className='pb-2 text-left font-medium'>
            {props.caption}
          </caption>
        )}
        <thead>
          <tr className='text-muted-foreground border-b text-left'>
            <th className='py-2 pr-3'>{t('Day')}</th>
            {props.dates.map((date, index) => (
              <th
                key={date}
                scope='col'
                className='py-2 pr-3 whitespace-nowrap'
              >
                {props.onDaySelect &&
                !singleDay &&
                !props.futureFlags[index] &&
                !props.unpublishedFlags?.[index] ? (
                  <button
                    type='button'
                    className='underline underline-offset-2'
                    onClick={() => props.onDaySelect?.(date)}
                  >
                    {date}
                  </button>
                ) : (
                  date
                )}
                {props.incompleteDays?.includes(date) && (
                  <span className='text-muted-foreground block text-xs'>
                    {t('Partial data')}
                  </span>
                )}
              </th>
            ))}
            {!singleDay && <th className='py-2'>{t('Period total')}</th>}
          </tr>
        </thead>
        <tbody>
          {[
            {
              label: t('Calls'),
              render: (m: UsageAnalyticsMetrics) => (
                <UsageCallsCell metrics={m} />
              ),
            },
            {
              label: t('Total tokens'),
              render: (m: UsageAnalyticsMetrics) => (
                <UsageTokensCell metrics={m} />
              ),
            },
            {
              label: t('Non-Token usage'),
              render: (m: UsageAnalyticsMetrics) => (
                <UsageNonTokenCell metrics={m} />
              ),
            },
            {
              label: t('Amount'),
              render: (m: UsageAnalyticsMetrics) => (
                <UsageMoneyCell metrics={m} />
              ),
            },
            {
              label: t('List price'),
              render: (m: UsageAnalyticsMetrics) =>
                m.original_quota_estimate == null
                  ? t('Cannot calculate')
                  : formatCustomerStatementQuota(m.original_quota_estimate),
            },
            {
              label: t('Reference amount'),
              render: (m: UsageAnalyticsMetrics) =>
                props.upstream ? (
                  <UsageReferenceAmount metrics={m} />
                ) : (
                  formatCustomerStatementQuota(m.reference_amount)
                ),
            },
            {
              label: t('Notes'),
              render: (m: UsageAnalyticsMetrics) => (
                <UsageQualityCell metrics={m} />
              ),
            },
          ]
            .filter((row) => {
              if (props.upstream) {
                return (
                  row.label !== t('Amount') && row.label !== t('List price')
                )
              }
              return (
                row.label !== t('Reference amount') ||
                props.total.reference_amount != null
              )
            })
            .map((row) => (
              <tr key={row.label} className='border-b align-top'>
                <th scope='row' className='py-2 pr-3 text-left'>
                  {row.label}
                </th>
                {props.days.map((m, i) => {
                  let status: string | undefined
                  if (props.futureFlags[i]) {
                    status = t('Not started')
                  } else if (props.unpublishedFlags?.[i]) {
                    status = t('Not published')
                  }
                  return (
                    <td key={props.dates[i]} className='py-2 pr-3'>
                      {status ? (
                        <span className='text-muted-foreground'>{status}</span>
                      ) : (
                        row.render(m)
                      )}
                    </td>
                  )
                })}
                {!singleDay && (
                  <td className='py-2'>{row.render(props.total)}</td>
                )}
              </tr>
            ))}
        </tbody>
      </table>
    </div>
  )
}
