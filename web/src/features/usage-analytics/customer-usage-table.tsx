import { useTranslation } from 'react-i18next'

import { formatCustomerStatementQuota } from '@/features/billing-reconciliation/lib'

import {
  UsageCallsCell,
  UsageMoneyCell,
  UsageQualityCell,
  UsageTokensCell,
} from './metrics-cells'
import type {
  UsageAnalyticsMetrics,
  UsageAnalyticsPeriod,
  UsageCustomerView,
} from './types'

// One row per API Key × customer model, with the range total; a collapsible
// section repeats the same leaves per day so weekly rows keep all seven days.
export function CustomerUsageTable(props: {
  view: UsageCustomerView
  period: UsageAnalyticsPeriod
}) {
  const { t } = useTranslation()
  return (
    <div className='flex flex-col gap-4'>
      {props.view.keys.length === 0 && (
        <p className='text-muted-foreground'>{t('No usage records')}</p>
      )}
      <div className='overflow-x-auto'>
        <table className='w-full min-w-[720px] text-sm'>
          <thead>
            <tr className='text-muted-foreground border-b text-left'>
              <th className='py-2 pr-3'>{t('API Key')}</th>
              <th className='py-2 pr-3'>{t('Customer model')}</th>
              <th className='py-2 pr-3'>{t('Calls')}</th>
              <th className='py-2 pr-3'>{t('Usage')}</th>
              <th className='py-2 pr-3'>{t('Amount')}</th>
              <th className='py-2 pr-3'>{t('Original estimate')}</th>
              <th className='py-2'>{t('Notes')}</th>
            </tr>
          </thead>
          <tbody>
            {props.view.keys.map((key) =>
              key.models.map((model, modelIndex) => (
                <tr
                  key={`${key.token_id}-${model.model_name === 'unknown' ? t('Unknown model') : model.model_name}`}
                  className='border-b align-top'
                >
                    <td className='py-2 pr-3'>
                      {modelIndex === 0 && (
                        <div className='flex flex-col'>
                          <span>
                            {key.token_id === 0
                              ? t('No API Key')
                              : key.token_name}
                          </span>
                          {key.token_deleted && (
                            <span className='text-muted-foreground text-xs'>
                              {t('Deleted key')}
                            </span>
                          )}
                        </div>
                      )}
                    </td>
                    <td className='py-2 pr-3'>
                      {model.model_name === 'unknown'
                        ? t('Unknown model')
                        : model.model_name}
                    </td>
                    <td className='py-2 pr-3'>
                      <UsageCallsCell metrics={model.total} />
                    </td>
                    <td className='py-2 pr-3'>
                      <UsageTokensCell metrics={model.total} />
                    </td>
                    <td className='py-2 pr-3'>
                      <UsageMoneyCell metrics={model.total} />
                    </td>
                    <td className='py-2 pr-3'>
                      {formatCustomerStatementQuota(
                        model.total.original_quota_estimate
                      )}
                      {model.total.multiple_discounts && (
                        <div className='text-muted-foreground text-xs'>
                          {t('Multiple discounts')}
                        </div>
                      )}
                    </td>
                    <td className='py-2'>
                      <UsageQualityCell metrics={model.total} />
                    </td>
                </tr>
              ))
            )}
            <tr className='font-medium'>
              <td className='py-2 pr-3'>{t('Total')}</td>
              <td />
              <td className='py-2 pr-3'>
                <UsageCallsCell metrics={props.view.total} />
              </td>
              <td className='py-2 pr-3'>
                <UsageTokensCell metrics={props.view.total} />
              </td>
              <td className='py-2 pr-3'>
                <UsageMoneyCell metrics={props.view.total} />
              </td>
              <td className='py-2 pr-3'>
                {formatCustomerStatementQuota(
                  props.view.total.original_quota_estimate
                )}
              </td>
              <td className='py-2'>
                <UsageQualityCell metrics={props.view.total} />
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <DayTotalsTable
        dates={props.period.days.map((day) => day.date)}
        futureFlags={props.period.days.map((day) => Boolean(day.future))}
        days={props.view.day_totals}
        total={props.view.total}
      />
    </div>
  )
}

export function DayTotalsTable(props: {
  dates: string[]
  futureFlags: boolean[]
  days: UsageAnalyticsMetrics[]
  total: UsageAnalyticsMetrics
}) {
  const { t } = useTranslation()
  // 单日模式下合计列与唯一日期列完全重复，按展示约定隐藏；周视图保留。
  const singleDay = props.dates.length === 1
  return (
    <div className='overflow-x-auto'>
      <table className='w-full min-w-[640px] text-sm'>
        <thead>
          <tr className='text-muted-foreground border-b text-left'>
            <th className='py-2 pr-3'>{t('Day')}</th>
            {props.dates.map((date) => (
              <th key={date} className='py-2 pr-3'>
                {date}
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
              label: t('Usage'),
              render: (m: UsageAnalyticsMetrics) => (
                <UsageTokensCell metrics={m} />
              ),
            },
            {
              label: t('Amount'),
              render: (m: UsageAnalyticsMetrics) => (
                <UsageMoneyCell metrics={m} />
              ),
            },
            {
              label: t('Original estimate'),
              render: (m: UsageAnalyticsMetrics) =>
                formatCustomerStatementQuota(m.original_quota_estimate),
            },
            {
              label: t('Reference amount'),
              render: (m: UsageAnalyticsMetrics) =>
                formatCustomerStatementQuota(m.reference_amount),
            },
            {
              label: t('Notes'),
              render: (m: UsageAnalyticsMetrics) => (
                <UsageQualityCell metrics={m} />
              ),
            },
          ]
            .filter(
              (row) =>
                row.label !== t('Reference amount') ||
                props.total.reference_amount != null
            )
            .map((row) => (
              <tr key={row.label} className='border-b align-top'>
                <th scope='row' className='py-2 pr-3 text-left'>
                  {row.label}
                </th>
                {props.days.map((m, i) => (
                  <td key={props.dates[i]} className='py-2 pr-3'>
                    {props.futureFlags[i] ? (
                      <span className='text-muted-foreground'>
                        {t('Not started')}
                      </span>
                    ) : (
                      row.render(m)
                    )}
                  </td>
                ))}
                {!singleDay && <td className='py-2'>{row.render(props.total)}</td>}
              </tr>
            ))}
        </tbody>
      </table>
    </div>
  )
}
