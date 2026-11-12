import { Fragment, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Toggle } from '@/components/ui/toggle'
import { formatCustomerStatementQuota } from '@/features/billing-reconciliation/lib'

import { UsageDiscountCells } from './discount-details'
import {
  UsageCallsCell,
  UsageMoneyCell,
  UsageNonTokenCell,
  UsageReferenceAmount,
  UsageQualityCell,
  UsageTokensCell,
} from './metrics-cells'
import type {
  UsageAnalyticsMetrics,
  UsageAnalyticsPeriod,
  UsageCustomerView,
} from './types'

// Week views show seven dates at each key/model level in full-width rows.
export function CustomerUsageTable(props: {
  view: UsageCustomerView
  period: UsageAnalyticsPeriod
  modelName?: string
  onDaySelect?: (day: string) => void
}) {
  const { t } = useTranslation()
  const [showDiscounts, setShowDiscounts] = useState(false)
  const dates = props.period.days.map((day) => day.date)
  const futureFlags = props.period.days.map((day) => Boolean(day.future))
  const unpublishedFlags = props.period.days.map((day) =>
    Boolean(day.unpublished)
  )
  const weekly = props.period.period === 'week'
  const columnCount = showDiscounts ? 12 : 7
  return (
    <div className='flex flex-col gap-4'>
      {props.modelName && <h2 className='font-medium'>{props.modelName}</h2>}
      <Toggle
        variant='outline'
        size='sm'
        pressed={showDiscounts}
        onPressedChange={setShowDiscounts}
        className='self-start'
      >
        {t('Pricing and discounts')}
      </Toggle>
      <div className='overflow-x-auto'>
        <table
          className={
            showDiscounts
              ? 'w-full min-w-[1440px] text-sm'
              : 'w-full min-w-[640px] text-sm'
          }
        >
          <thead>
            <tr className='text-muted-foreground border-b text-left'>
              <th className='py-2 pr-3'>{t('API Key')}</th>
              <th className='py-2 pr-3'>{t('Customer model')}</th>
              <th className='py-2 pr-3'>{t('Calls')}</th>
              <th scope='col' className='py-2 pr-3'>
                {t('Total tokens')}
              </th>
              <th scope='col' className='py-2 pr-3'>
                {t('Non-Token usage')}
              </th>
              <th className='py-2 pr-3'>{t('Amount')}</th>
              {showDiscounts && (
                <>
                  <th className='py-2 pr-3'>{t('Original estimate')}</th>
                  <th className='py-2 pr-3'>{t('Group discount/ratio')}</th>
                  <th className='py-2 pr-3'>{t('Contract discount')}</th>
                  <th className='py-2 pr-3'>{t('Final discount')}</th>
                  <th className='py-2 pr-3'>{t('Estimated savings')}</th>
                </>
              )}
              <th className='py-2'>{t('Notes')}</th>
            </tr>
          </thead>
          <tbody>
            {props.view.keys
              .filter(
                (key) =>
                  !props.modelName ||
                  key.models.some(
                    (model) => model.model_name === props.modelName
                  )
              )
              .map((key) => (
                <Fragment key={key.token_id}>
                  {weekly && !props.modelName && (
                    <tr>
                      <td colSpan={columnCount} className='py-3'>
                        <DayTotalsTable
                          customer
                          showOriginalEstimate={showDiscounts}
                          caption={`${t('API Key')} · ${key.token_id === 0 ? t('No API Key') : key.token_name}`}
                          dates={dates}
                          futureFlags={futureFlags}
                          unpublishedFlags={unpublishedFlags}
                          days={key.days}
                          total={key.total}
                        />
                      </td>
                    </tr>
                  )}
                  {key.models
                    .filter(
                      (model) =>
                        !props.modelName || model.model_name === props.modelName
                    )
                    .map((model, modelIndex) => (
                      <Fragment key={model.model_name}>
                        <tr className='border-b align-top'>
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
                            <UsageNonTokenCell metrics={model.total} />
                          </td>
                          <td className='py-2 pr-3'>
                            <UsageMoneyCell
                              metrics={model.total}
                              describeFees
                            />
                          </td>
                          {showDiscounts && (
                            <>
                              <td className='py-2 pr-3'>
                                {formatCustomerStatementQuota(
                                  model.total.original_quota_estimate
                                )}
                              </td>
                              <UsageDiscountCells
                                combinations={model.discount_combinations}
                                moneyIncomplete={
                                  (model.total.rows_missing_money ?? 0) > 0 ||
                                  (model.total.rows_money_pending ?? 0) > 0
                                }
                              />
                            </>
                          )}
                          <td className='py-2'>
                            <UsageQualityCell metrics={model.total} />
                          </td>
                        </tr>
                        {weekly && (
                          <tr>
                            <td colSpan={columnCount} className='pb-4'>
                              <DayTotalsTable
                                customer
                                showOriginalEstimate={showDiscounts}
                                caption={`${t('Customer model')} · ${model.model_name === 'unknown' ? t('Unknown model') : model.model_name}`}
                                dates={dates}
                                futureFlags={futureFlags}
                                unpublishedFlags={unpublishedFlags}
                                days={model.days}
                                total={model.total}
                              />
                            </td>
                          </tr>
                        )}
                      </Fragment>
                    ))}
                </Fragment>
              ))}
            {!props.modelName && (
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
                  <UsageNonTokenCell metrics={props.view.total} />
                </td>
                <td className='py-2 pr-3'>
                  <UsageMoneyCell metrics={props.view.total} describeFees />
                </td>
                {showDiscounts && (
                  <>
                    <td className='py-2 pr-3'>
                      {formatCustomerStatementQuota(
                        props.view.total.original_quota_estimate
                      )}
                    </td>
                    <td className='py-2 pr-3'>—</td>
                    <td className='py-2 pr-3'>—</td>
                    <td className='py-2 pr-3'>—</td>
                    <td className='py-2 pr-3'>—</td>
                  </>
                )}
                <td className='py-2'>
                  <UsageQualityCell metrics={props.view.total} />
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
      {weekly && !props.modelName && (
        <DayTotalsTable
          customer
          showOriginalEstimate={showDiscounts}
          onDaySelect={props.onDaySelect}
          dates={dates}
          futureFlags={futureFlags}
          unpublishedFlags={unpublishedFlags}
          days={props.view.day_totals}
          total={props.view.total}
        />
      )}
    </div>
  )
}

export function DayTotalsTable(props: {
  customer?: boolean
  upstream?: boolean
  showOriginalEstimate?: boolean
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
                <UsageMoneyCell metrics={m} describeFees={props.customer} />
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
                  row.label !== t('Amount') &&
                  row.label !== t('Original estimate')
                )
              }
              if (row.label === t('Original estimate')) {
                return props.showOriginalEstimate !== false
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
