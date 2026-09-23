import { useMutation, useQuery } from '@tanstack/react-query'
import { Fragment, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { formatCustomerStatementQuota } from '@/features/billing-reconciliation/lib'

import {
  createUsageAdminExport,
  getAdminUsageCustomerSummary,
  getAdminUsageCustomers,
  getAdminUsageUpstreamSummary,
} from './api'
import { CustomerUsageTable, DayTotalsTable } from './customer-usage-table'
import {
  UsageCallsCell,
  UsageTokensCell,
  UsageMoneyCell,
  UsageQualityCell,
} from './metrics-cells'
import { UsagePeriodPicker } from './period-picker'
import type { UsageAnalyticsPeriod, UsageUpstreamView } from './types'
import { shanghaiToday, formatNumber } from './utils'

// Admin day/week usage page with two tabs: customer usage (list + per-customer
// drill-down) and upstream usage. Upstream fields never appear in the customer
// tab payloads.
type AdminUsageSection = 'customers' | 'upstream'

export function AdminUsage(props: {
  period: 'day' | 'week'
  date?: string
  section?: AdminUsageSection
  onSearchChange: (search: {
    period: 'day' | 'week'
    date: string
    section?: AdminUsageSection
  }) => void
}) {
  const { t } = useTranslation()
  const date = props.date ?? shanghaiToday()
  const setSection = (section: string) =>
    props.onSearchChange({
      period: props.period,
      date,
      section: section as AdminUsageSection,
    })
  return (
    <div className='flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto p-4'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <UsagePeriodPicker
          period={props.period}
          date={date}
          resolved={undefined}
          onPeriodChange={(period) =>
            props.onSearchChange({ period, date, section: props.section })
          }
          onDateChange={(nextDate) =>
            props.onSearchChange({
              period: props.period,
              date: nextDate,
              section: props.section,
            })
          }
        />
        {props.section === 'upstream' && (
          <AdminExportButton
            body={{ period: props.period, date, view: 'upstream' }}
          />
        )}
      </div>
      <p className='text-muted-foreground text-sm'>
        {t('Usage coverage notice')}
      </p>
      <Tabs value={props.section ?? 'customers'} onValueChange={setSection}>
        <TabsList>
          <TabsTrigger value='customers'>{t('Customer usage')}</TabsTrigger>
          <TabsTrigger value='upstream'>{t('Upstream usage')}</TabsTrigger>
        </TabsList>
        <TabsContent value='customers'>
          <CustomersTab period={props.period} date={date} />
        </TabsContent>
        <TabsContent value='upstream'>
          <UpstreamTab period={props.period} date={date} />
        </TabsContent>
      </Tabs>
    </div>
  )
}

// Admin export submission; files are delivered through the shared billing
// export task drawer after the queue finishes generation.
function AdminExportButton(props: {
  body: {
    period: 'day' | 'week'
    date: string
    view: 'customer' | 'customers' | 'upstream'
    user_id?: number
    search?: string
  }
}) {
  const { t, i18n } = useTranslation()
  const mutation = useMutation({
    mutationFn: async () => {
      const response = await createUsageAdminExport({
        ...props.body,
        language: i18n.language.startsWith('zh') ? 'zh' : 'en',
      })
      if (!response.success) {
        throw new Error(response.message)
      }
      return response.data
    },
    onSuccess: () => {
      toast.success(
        t('Export job queued; check billing export tasks for the file')
      )
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })
  return (
    <Button
      size='sm'
      disabled={mutation.isPending}
      onClick={() => mutation.mutate()}
    >
      {t('Export CSV')}
    </Button>
  )
}

function CustomersTab(props: { period: 'day' | 'week'; date: string }) {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  const list = useQuery({
    queryKey: ['usage-admin-customers', props.period, props.date, search],
    queryFn: () =>
      getAdminUsageCustomers({
        period: props.period,
        date: props.date,
        search: search || undefined,
      }),
  })
  const [selectedUser, setSelectedUser] = useState<number | null>(null)
  const detail = useQuery({
    queryKey: [
      'usage-admin-customer-summary',
      props.period,
      props.date,
      selectedUser,
    ],
    queryFn: () =>
      getAdminUsageCustomerSummary({
        period: props.period,
        date: props.date,
        user_id: selectedUser ?? 0,
      }),
    enabled: selectedUser !== null,
  })
  return (
    <div className='flex flex-col gap-4'>
      <div className='flex items-center gap-2'>
        <Input
          aria-label={t('Search customer')}
          placeholder={t('Search customer')}
          value={search}
          className='w-56'
          onChange={(event) => setSearch(event.target.value)}
        />
        <AdminExportButton
          body={{
            period: props.period,
            date: props.date,
            view: 'customers',
            search,
          }}
        />
      </div>
      {list.isLoading && <LoadingState />}
      {list.isError && <ErrorState />}
      {list.data && (
        <div className='overflow-x-auto'>
          {list.data.data.result.customers.length === 0 && (
            <p className='text-muted-foreground'>{t('No usage records')}</p>
          )}
          <table className='w-full min-w-[640px] text-sm'>
            <thead>
              <tr className='text-muted-foreground border-b text-left'>
                <th className='py-2 pr-3'>{t('Customer')}</th>
                <th className='py-2 pr-3'>{t('Calls')}</th>
                <th className='py-2 pr-3'>{t('Usage')}</th>
                <th className='py-2 pr-3'>{t('Amount')}</th>
                <th className='py-2'>{t('Original estimate')}</th>
              </tr>
            </thead>
            <tbody>
              {list.data.data.result.customers.map((customer) => (
                <tr
                  key={customer.user_id}
                  className='hover:bg-muted/40 border-b'
                >
                  <td className='py-2 pr-3'>
                    <Button
                      variant='link'
                      onClick={() => setSelectedUser(customer.user_id)}
                    >
                      {customer.display_name || customer.username}
                    </Button>
                    {customer.deleted && (
                      <span className='text-muted-foreground ml-1 text-xs'>
                        {t('Deleted')}
                      </span>
                    )}
                    <span className='text-muted-foreground ml-1 text-xs'>
                      #{customer.user_id}
                    </span>
                  </td>
                  <td className='py-2 pr-3'>
                    <UsageCallsCell metrics={customer.total} />
                  </td>
                  <td className='py-2 pr-3'>
                    <UsageTokensCell metrics={customer.total} />
                  </td>
                  <td className='py-2 pr-3'>
                    <UsageMoneyCell metrics={customer.total} />
                  </td>
                  <td className='py-2'>
                    {formatCustomerStatementQuota(
                      customer.total.original_quota_estimate
                    )}
                  </td>
                </tr>
              ))}
              <tr className='font-medium'>
                <td className='py-2 pr-3'>{t('Total')}</td>
                <td className='py-2 pr-3'>
                  {formatNumber(list.data.data.result.total.total_calls)}
                </td>
                <td className='py-2 pr-3'>
                  {formatNumber(list.data.data.result.total.input_tokens)} /{' '}
                  {formatNumber(list.data.data.result.total.output_tokens)}
                </td>
                <td className='py-2 pr-3'>
                  <UsageMoneyCell metrics={list.data.data.result.total} />
                </td>
                <td className='py-2'>
                  {formatCustomerStatementQuota(
                    list.data.data.result.total.original_quota_estimate
                  )}
                </td>
              </tr>
            </tbody>
          </table>
          {list.data.data.result.truncated && (
            <p className='text-muted-foreground mt-2 text-xs'>
              {t('Result truncated at the aggregation limit')}
            </p>
          )}
        </div>
      )}
      {selectedUser !== null && (
        <div className='flex flex-col gap-2 rounded-md border p-3'>
          <div className='flex items-center justify-between'>
            <span className='text-sm font-medium'>
              {t('Customer #{{id}} usage', { id: selectedUser })}
            </span>
            <div className='flex items-center gap-2'>
              <AdminExportButton
                body={{
                  period: props.period,
                  date: props.date,
                  view: 'customer',
                  user_id: selectedUser,
                }}
              />
              <Button
                variant='outline'
                size='sm'
                onClick={() => setSelectedUser(null)}
              >
                {t('Close')}
              </Button>
            </div>
          </div>
          {detail.isLoading && <LoadingState />}
          {detail.isError && <ErrorState />}
          {detail.data && (
            <CustomerUsageTable
              view={detail.data.data.result}
              period={detail.data.data.period as UsageAnalyticsPeriod}
            />
          )}
        </div>
      )}
    </div>
  )
}

function UpstreamTab(props: { period: 'day' | 'week'; date: string }) {
  const query = useQuery({
    queryKey: ['usage-admin-upstream', props.period, props.date],
    queryFn: () =>
      getAdminUsageUpstreamSummary({ period: props.period, date: props.date }),
  })
  if (query.isLoading) {
    return <LoadingState />
  }
  if (query.isError || !query.data) {
    return <ErrorState />
  }
  return (
    <UpstreamViewGroups
      view={query.data.data.result}
      period={query.data.data.period}
    />
  )
}

export function UpstreamViewGroups(props: {
  view: UsageUpstreamView
  period: UsageAnalyticsPeriod
}) {
  const { t } = useTranslation()
  const dates = props.period.days.map((day) => day.date)
  const futureFlags = props.period.days.map((day) => Boolean(day.future))
  const weekly = props.period.period === 'week'
  return (
    <div className='flex flex-col gap-4'>
      <DayTotalsTable
        dates={dates}
        futureFlags={futureFlags}
        days={props.view.day_totals}
        total={props.view.total}
      />
      {props.view.url_groups.length === 0 && (
        <p className='text-muted-foreground'>{t('No usage records')}</p>
      )}
      {props.view.url_groups.map((group) => (
        <section
          key={group.url_key}
          className='space-y-3 rounded-md border p-3'
        >
          <h2 className='font-medium'>{group.display_name}</h2>
          {group.unidentified && (
            <p className='text-muted-foreground text-sm'>
              {t('Unidentified URL grouping')}
            </p>
          )}
          {weekly && (
            <DayTotalsTable
              caption={group.display_name}
              dates={dates}
              futureFlags={futureFlags}
              days={group.days}
              total={group.total}
            />
          )}
          {group.models.map((model) => (
            <section
              key={`${model.provider_model}-${model.billing_mode}-${model.provider_model_fallback}`}
              className='space-y-2 rounded-md border p-3'
            >
              <h3 className='font-medium'>
                {model.provider_model === 'unknown'
                  ? t('Unknown model')
                  : model.provider_model}{' '}
                · {model.billing_mode}
              </h3>
              {model.provider_model_fallback && (
                <p className='text-muted-foreground text-xs'>
                  {t('Provider model unrecorded')}
                </p>
              )}
              <div className='flex flex-wrap gap-6'>
                <UsageCallsCell metrics={model.total} />
                <UsageTokensCell metrics={model.total} />
                <div className='text-sm'>
                  {t('Original estimate')}:{' '}
                  {formatCustomerStatementQuota(
                    model.total.original_quota_estimate
                  )}
                  <br />
                  {t('Reference amount')}:{' '}
                  {formatCustomerStatementQuota(model.total.reference_amount)}
                </div>
              </div>
              {weekly && (
                <DayTotalsTable
                  caption={`${t('Provider model')} · ${model.provider_model === 'unknown' ? t('Unknown model') : model.provider_model}`}
                  dates={dates}
                  futureFlags={futureFlags}
                  days={model.days}
                  total={model.total}
                />
              )}
              <Collapsible defaultOpen={weekly}>
                <CollapsibleTrigger className='rounded px-2 py-1 text-sm underline focus-visible:outline-2'>
                  {t('Channels')} ({model.channels.length})
                </CollapsibleTrigger>
                <CollapsibleContent>
                  <div className='overflow-x-auto'>
                    <table className='w-full min-w-[720px] text-sm'>
                      <thead>
                        <tr className='text-muted-foreground border-b text-left'>
                          <th className='p-2'>{t('Channel')}</th>
                          <th className='p-2'>{t('Calls')}</th>
                          <th className='p-2'>{t('Usage')}</th>
                          <th className='p-2'>{t('Original estimate')}</th>
                          <th className='p-2'>{t('Reference amount')}</th>
                          <th className='p-2'>{t('Channel discount')}</th>
                        </tr>
                      </thead>
                      <tbody>
                        {model.channels.map((channel) => (
                          <Fragment key={channel.channel_id}>
                            <tr className='border-b align-top'>
                              <td className='p-2'>{channel.channel_name}</td>
                              <td className='p-2'>
                                <UsageCallsCell metrics={channel.total} />
                              </td>
                              <td className='p-2'>
                                <UsageTokensCell metrics={channel.total} />
                              </td>
                              <td className='p-2'>
                                {formatCustomerStatementQuota(
                                  channel.total.original_quota_estimate
                                )}
                              </td>
                              <td className='p-2'>
                                {formatCustomerStatementQuota(
                                  channel.total.reference_amount
                                )}
                                <UsageQualityCell metrics={channel.total} />
                              </td>
                              <td className='p-2'>
                                <div className='flex flex-col gap-1 text-xs'>
                                  {channel.discounts.map((discount) => (
                                    <span key={discount.period_start}>
                                      {new Date(
                                        discount.period_start * 1000
                                      ).toLocaleDateString('en-CA', {
                                        timeZone: 'Asia/Shanghai',
                                        year: 'numeric',
                                        month: '2-digit',
                                      })}
                                      : {discount.value} (
                                      {discount.source === 'default'
                                        ? t('Default')
                                        : t('Configured')}
                                      )
                                    </span>
                                  ))}
                                  {channel.total.multiple_discounts && (
                                    <span>{t('Multiple discounts')}</span>
                                  )}
                                  {channel.usage_only && (
                                    <span>{t('Unpriced channel tests')}</span>
                                  )}
                                  {(channel.total.test_priced_rows ?? 0) >
                                    0 && (
                                    <span>
                                      {t(
                                        '{{count}} channel tests included in the reference amount.',
                                        {
                                          count: channel.total.test_priced_rows,
                                        }
                                      )}
                                    </span>
                                  )}
                                </div>
                              </td>
                            </tr>
                            {weekly && (
                              <tr>
                                <td colSpan={6} className='p-2 pb-4'>
                                  <DayTotalsTable
                                    caption={`${t('Channel')} · ${channel.channel_name}`}
                                    dates={dates}
                                    futureFlags={futureFlags}
                                    days={channel.days}
                                    total={channel.total}
                                  />
                                </td>
                              </tr>
                            )}
                          </Fragment>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </CollapsibleContent>
              </Collapsible>
            </section>
          ))}
        </section>
      ))}
    </div>
  )
}
