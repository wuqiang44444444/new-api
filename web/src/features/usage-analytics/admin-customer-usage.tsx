import { useQuery } from '@tanstack/react-query'
import { useId, useLayoutEffect, useRef, useState, type RefObject } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Combobox } from '@/components/ui/combobox'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { formatCustomerStatementQuota } from '@/features/billing-reconciliation/lib'
import { useDebounce } from '@/hooks/use-debounce'

import { getAdminUsageCustomerSummary, getAdminUsageCustomers } from './api'
import { UsageExportButton } from './export-button'
import {
  UsageCallsCell,
  UsageMoneyCell,
  UsageNonTokenCell,
  UsageTokensCell,
} from './metrics-cells'
import { PersonalUsageOverview } from './personal-usage-overview'
import type { UsageCustomerOverviewRow, UsagePeriodKind } from './types'
import { UsageCustomerReport } from './usage-customer-report'
import { UsageDailyBreakdown } from './usage-daily-breakdown'
import { PublishedUsageRanking } from './usage-rankings'
import { UsageReportEmpty } from './usage-report-empty'
import { UsageReportStatus } from './usage-report-status'
import { formatNumber, usageReportDayQuery } from './utils'

export function AdminCustomerUsage(props: {
  period: UsagePeriodKind
  date: string
  reportDays?: string
  scrollContainer: RefObject<HTMLDivElement | null>
  onDaySelect: (day: string, reportDays?: string) => void
}) {
  const { t } = useTranslation()
  const listId = useId()
  const customerFilterId = useId()
  const detailHeading = useRef<HTMLHeadingElement>(null)
  const listHeading = useRef<HTMLHeadingElement>(null)
  const returnTo = useRef<{ id: number; scrollTop: number } | undefined>(
    undefined
  )
  const [selectedUser, setSelectedUser] =
    useState<UsageCustomerOverviewRow | null>(null)
  const [modelName, setModelName] = useState<string>()
  const [search, setSearch] = useState('')
  const filter = useDebounce(search.trim(), 300)
  const [sort, setSort] = useState('amount')
  const [listOpen, setListOpen] = useState(false)
  const params = {
    period: props.period,
    date: props.date,
    report_days: props.reportDays,
  }
  const overview = useQuery({
    queryKey: [
      'usage-admin-customers',
      props.period,
      props.date,
      props.reportDays,
    ],
    queryFn: ({ signal }) =>
      getAdminUsageCustomers(
        { ...params, compact: true, include_days: props.period !== 'day' },
        signal
      ),
  })
  const globalReport = overview.data?.data
  const reportDays = globalReport?.report_days?.length
    ? JSON.stringify(globalReport.report_days)
    : props.reportDays
  const filtered = useQuery({
    queryKey: [
      'usage-admin-customers-filtered',
      props.period,
      props.date,
      filter,
      reportDays,
    ],
    queryFn: ({ signal }) =>
      getAdminUsageCustomers(
        { ...params, search: filter, report_days: reportDays, compact: true },
        signal
      ),
    enabled: listOpen && !selectedUser && !!filter && !overview.isPending,
  })
  const list = filter ? filtered : overview
  const report = list.data?.data
  const detailDays =
    reportDays ??
    (report?.report_days?.length
      ? JSON.stringify(report.report_days)
      : undefined)
  const detail = useQuery({
    queryKey: [
      'usage-admin-customer-summary',
      props.period,
      props.date,
      selectedUser?.user_id,
      detailDays,
    ],
    queryFn: ({ signal }) =>
      getAdminUsageCustomerSummary(
        {
          ...params,
          user_id: selectedUser?.user_id ?? 0,
          report_days: detailDays,
        },
        signal
      ),
    enabled: selectedUser != null && !overview.isPending,
  })
  const openCustomer = (customer: UsageCustomerOverviewRow) => {
    returnTo.current = {
      id: customer.user_id,
      scrollTop: props.scrollContainer.current?.scrollTop ?? 0,
    }
    setSelectedUser(customer)
    setModelName(undefined)
  }
  const customers = new Map(
    [
      ...(globalReport?.result.customers ?? []),
      ...(report?.result.customers ?? []),
    ].map((customer) => [String(customer.user_id), customer])
  )
  if (selectedUser && !customers.has(String(selectedUser.user_id))) {
    customers.set(String(selectedUser.user_id), selectedUser)
  }
  const customerFilter = (
    <Combobox
      id={customerFilterId}
      aria-label={t('Customer')}
      className='w-60 max-w-full'
      value={selectedUser ? String(selectedUser.user_id) : ''}
      options={[
        { value: '', label: t('All customers') },
        ...Array.from(customers, ([value, customer]) => ({
          value,
          label: `${customer.display_name || customer.username} · #${customer.user_id}`,
        })),
      ]}
      onValueChange={(value) => {
        const customer = value ? customers.get(value) : undefined
        if (customer) openCustomer(customer)
        else setSelectedUser(null)
      }}
    />
  )
  useLayoutEffect(() => {
    if (selectedUser) {
      if (props.scrollContainer.current) {
        props.scrollContainer.current.scrollTop = 0
      }
      detailHeading.current?.focus({ preventScroll: true })
    } else if (returnTo.current) {
      const target = returnTo.current
      returnTo.current = undefined
      const trigger =
        props.scrollContainer.current?.querySelector<HTMLButtonElement>(
          `[id="${listId}-${target.id}"]`
        ) ??
        props.scrollContainer.current?.querySelector<HTMLInputElement>(
          `[id="${customerFilterId}"]`
        ) ??
        listHeading.current
      trigger?.focus({ preventScroll: true })
      if (props.scrollContainer.current) {
        props.scrollContainer.current.scrollTop = target.scrollTop
      }
    }
  }, [selectedUser, listId, customerFilterId, props.scrollContainer])

  if (selectedUser) {
    const customerName = selectedUser.display_name || selectedUser.username
    const customerReport = detail.data?.data
    const selectedModel = customerReport?.result.keys.some((key) =>
      key.models.some((model) => model.model_name === modelName)
    )
      ? modelName
      : undefined
    return (
      <section className='flex flex-col gap-4'>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <div className='flex flex-wrap items-center gap-3'>
            <Button
              variant='outline'
              size='sm'
              onClick={() => setSelectedUser(null)}
            >
              {t('Back to customer usage')}
            </Button>
            <h2
              ref={detailHeading}
              tabIndex={-1}
              className='text-base font-semibold wrap-anywhere'
            >
              {customerName} · #{selectedUser.user_id}
              {selectedUser.deleted && (
                <span className='text-muted-foreground ms-2 text-xs'>
                  {t('Deleted')}
                </span>
              )}
            </h2>
          </div>
          <div className='flex flex-wrap items-center gap-2'>
            {customerFilter}
            <UsageExportButton
              customerName={customerName}
              ready={
                detail.isSuccess &&
                !detail.isFetching &&
                (customerReport?.published_days?.length ?? 1) > 0
              }
              period={customerReport?.period}
              customerView={customerReport?.result}
              body={{
                period: props.period,
                date: props.date,
                view: 'customer',
                report_days: customerReport?.report_days,
                user_id: selectedUser.user_id,
                model_name: selectedModel,
              }}
            />
          </div>
        </div>
        {detail.isLoading && <LoadingState />}
        {detail.isError && <ErrorState onRetry={() => void detail.refetch()} />}
        {customerReport && (
          <UsageCustomerReport
            key={`${selectedUser.user_id}:${props.period}:${props.date}`}
            report={customerReport}
            modelName={selectedModel}
            adminCharts
            onModelChange={setModelName}
            onDaySelect={(day) =>
              props.onDaySelect(day, usageReportDayQuery(customerReport, day))
            }
          />
        )}
      </section>
    )
  }

  const sortedCustomers = (
    listOpen ? [...(report?.result.customers ?? [])] : []
  ).sort((a, b) => {
    if (sort === 'calls') {
      return b.total.total_calls - a.total.total_calls || a.user_id - b.user_id
    }
    if (sort === 'tokens') {
      const aTokens = (a.total.input_tokens ?? 0) + (a.total.output_tokens ?? 0)
      const bTokens = (b.total.input_tokens ?? 0) + (b.total.output_tokens ?? 0)
      return bTokens - aTokens || a.user_id - b.user_id
    }
    return (
      (b.total.net_quota ?? 0) - (a.total.net_quota ?? 0) ||
      a.user_id - b.user_id
    )
  })
  const sortItems = [
    { value: 'amount', label: t('Net amount') },
    { value: 'calls', label: t('Calls') },
    { value: 'tokens', label: t('Total tokens') },
  ]
  return (
    <div className='flex flex-col gap-4'>
      <section aria-labelledby={`${listId}-overview`} className='space-y-3'>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <h2 id={`${listId}-overview`} className='text-base font-semibold'>
            {t('All-customer usage')}
          </h2>
          <div className='flex flex-wrap items-center gap-2'>
            {customerFilter}
            <UsageExportButton
              ready={
                overview.isSuccess &&
                !overview.isFetching &&
                (globalReport?.published_days?.length ?? 1) > 0
              }
              period={globalReport?.period}
              body={{
                period: props.period,
                date: props.date,
                view: 'customers',
                report_days: globalReport?.report_days,
              }}
            />
          </div>
        </div>
        <UsageReportStatus report={globalReport ?? report} showReportPeriod />
        {overview.isLoading && <LoadingState />}
        {overview.isError && (
          <ErrorState
            title={t('Unable to load all-customer usage.')}
            onRetry={() => void overview.refetch()}
          />
        )}
        {globalReport &&
          (globalReport.published_days?.length ?? 1) > 0 &&
          globalReport.result.customers.length > 0 && (
            <>
              <PersonalUsageOverview metrics={globalReport.result.total} />
              <PublishedUsageRanking
                key={`${props.period}:${props.date}:${reportDays ?? ''}`}
                kind='model'
                params={{ ...params, report_days: reportDays }}
              />
              {props.period !== 'day' && (
                <UsageDailyBreakdown
                  period={globalReport.period}
                  days={globalReport.result.day_totals}
                  incompleteDays={globalReport.incomplete_days}
                  onDaySelect={(day) =>
                    props.onDaySelect(
                      day,
                      usageReportDayQuery(globalReport, day)
                    )
                  }
                />
              )}
            </>
          )}
        {globalReport &&
          (globalReport.published_days?.length ?? 1) > 0 &&
          !globalReport.result.customers.length && (
            <UsageReportEmpty period={globalReport.period} />
          )}
      </section>
      <Collapsible open={listOpen} onOpenChange={setListOpen}>
        <CollapsibleTrigger render={<Button size='sm' variant='outline' />}>
          {t('Customer list')}
        </CollapsibleTrigger>
        <CollapsibleContent className='mt-3'>
          <section
            aria-labelledby={`${listId}-customers`}
            className='space-y-3'
          >
            <div className='flex flex-wrap items-center justify-between gap-2'>
              <h2
                id={`${listId}-customers`}
                ref={listHeading}
                tabIndex={-1}
                className='text-base font-semibold'
              >
                {filter ? t('Matching customers') : t('Customers')}
              </h2>
              <div className='flex flex-wrap items-center gap-2'>
                <Input
                  aria-label={t('Search customer')}
                  placeholder={t('Search customer')}
                  value={search}
                  className='w-56 max-w-full'
                  onChange={(event) => setSearch(event.target.value)}
                />
                <Select
                  value={sort}
                  onValueChange={(value) => {
                    if (value) setSort(value)
                  }}
                  items={sortItems}
                >
                  <SelectTrigger aria-label={t('Sort customers by')}>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {sortItems.map((item) => (
                      <SelectItem key={item.value} value={item.value}>
                        {item.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {!!filter && (
                  <UsageExportButton
                    triggerLabel={t('Export search results')}
                    ready={
                      list.isSuccess &&
                      !list.isFetching &&
                      filter === search.trim() &&
                      (report?.published_days?.length ?? 1) > 0
                    }
                    period={report?.period}
                    body={{
                      period: props.period,
                      date: props.date,
                      view: 'customers',
                      report_days: report?.report_days,
                      search: filter,
                    }}
                  />
                )}
              </div>
            </div>
            {!!filter && list.isLoading && <LoadingState />}
            {!!filter && list.isError && (
              <ErrorState onRetry={() => void list.refetch()} />
            )}
            {report &&
              (report.published_days?.length ?? 1) > 0 &&
              report.result.customers.length === 0 &&
              !!filter && (
                <EmptyState bordered title={t('No matching results')} />
              )}
            {listOpen && report && (
              <div className='overflow-x-auto'>
                {report.result.customers.length > 0 && (
                  <table className='w-full min-w-[640px] text-sm'>
                    <thead>
                      <tr className='text-muted-foreground border-b text-left'>
                        <th className='py-2 pr-3'>{t('Customer')}</th>
                        <th className='py-2 pr-3'>{t('Calls')}</th>
                        <th scope='col' className='py-2 pr-3'>
                          {t('Total tokens')}
                        </th>
                        <th scope='col' className='py-2 pr-3'>
                          {t('Non-Token usage')}
                        </th>
                        <th className='py-2 pr-3'>{t('Net amount')}</th>
                        <th className='py-2'>{t('Original estimate')}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {sortedCustomers.map((customer) => (
                        <tr
                          key={customer.user_id}
                          className='hover:bg-muted/40 border-b'
                        >
                          <td className='py-2 pr-3'>
                            <Button
                              variant='link'
                              id={`${listId}-${customer.user_id}`}
                              onClick={() => openCustomer(customer)}
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
                            <UsageNonTokenCell metrics={customer.total} />
                          </td>
                          <td className='py-2 pr-3'>
                            <UsageMoneyCell
                              metrics={customer.total}
                              describeFees
                            />
                          </td>
                          <td className='py-2'>
                            {formatCustomerStatementQuota(
                              customer.total.original_quota_estimate
                            )}
                          </td>
                        </tr>
                      ))}
                      <tr className='font-medium'>
                        <td className='py-2 pr-3'>
                          {filter ? t('Filtered total') : t('Total')}
                        </td>
                        <td className='py-2 pr-3'>
                          {formatNumber(report.result.total.total_calls)}
                        </td>
                        <td className='py-2 pr-3'>
                          <UsageTokensCell metrics={report.result.total} />
                        </td>
                        <td className='py-2 pr-3'>
                          <UsageNonTokenCell metrics={report.result.total} />
                        </td>
                        <td className='py-2 pr-3'>
                          <UsageMoneyCell
                            metrics={report.result.total}
                            describeFees
                          />
                        </td>
                        <td className='py-2'>
                          {formatCustomerStatementQuota(
                            report.result.total.original_quota_estimate
                          )}
                        </td>
                      </tr>
                    </tbody>
                  </table>
                )}

                {report.result.truncated && (
                  <p className='text-muted-foreground mt-2 text-xs'>
                    {t('Result truncated at the aggregation limit')}
                  </p>
                )}
              </div>
            )}
          </section>
        </CollapsibleContent>
      </Collapsible>
    </div>
  )
}
