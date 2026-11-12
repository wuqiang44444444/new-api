import { useQuery } from '@tanstack/react-query'
import { useId, useLayoutEffect, useRef, useState, type RefObject } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'

import { getAdminUsageCustomerSummary, getAdminUsageCustomers } from './api'
import { UsageExportButton } from './export-button'
import { PersonalUsageOverview } from './personal-usage-overview'
import type { UsageCustomerOverviewRow, UsagePeriodKind } from './types'
import { UsageCustomerReport } from './usage-customer-report'
import { PublishedUsageRanking } from './usage-rankings'
import { UsageReportEmpty } from './usage-report-empty'
import { UsageReportStatus } from './usage-report-status'

export function AdminCustomerUsage(props: {
  period: UsagePeriodKind
  date: string
  scrollContainer: RefObject<HTMLDivElement | null>
}) {
  const { t } = useTranslation()
  const overviewHeadingId = useId()
  const detailHeading = useRef<HTMLHeadingElement>(null)
  const customerFilterInput = useRef<HTMLInputElement>(null)
  const overviewScrollTop = useRef<number | undefined>(undefined)
  const [selectedUser, setSelectedUser] =
    useState<UsageCustomerOverviewRow | null>(null)
  const params = {
    period: props.period,
    date: props.date,
  }
  const overview = useQuery({
    queryKey: ['usage-admin-customers', props.period, props.date],
    queryFn: ({ signal }) =>
      getAdminUsageCustomers({ ...params, compact: true }, signal),
  })
  const globalReport = overview.data?.data
  const reportDays = globalReport?.report_days?.length
    ? JSON.stringify(globalReport.report_days)
    : undefined
  const detail = useQuery({
    queryKey: [
      'usage-admin-customer-summary',
      props.period,
      props.date,
      selectedUser?.user_id,
      reportDays,
    ],
    queryFn: ({ signal }) =>
      getAdminUsageCustomerSummary(
        {
          ...params,
          user_id: selectedUser?.user_id ?? 0,
          report_days: reportDays,
        },
        signal
      ),
    enabled: selectedUser != null && !overview.isPending,
  })
  const customers = new Map(
    (globalReport?.result.customers ?? []).map((customer) => [
      String(customer.user_id),
      customer,
    ])
  )
  if (selectedUser && !customers.has(String(selectedUser.user_id))) {
    customers.set(String(selectedUser.user_id), selectedUser)
  }
  const customerFilter = (
    <Combobox
      ref={customerFilterInput}
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
        if (!customer) {
          setSelectedUser(null)
          return
        }
        if (!selectedUser) {
          overviewScrollTop.current =
            props.scrollContainer.current?.scrollTop ?? 0
        }
        setSelectedUser(customer)
      }}
    />
  )
  useLayoutEffect(() => {
    if (selectedUser) {
      if (props.scrollContainer.current) {
        props.scrollContainer.current.scrollTop = 0
      }
      detailHeading.current?.focus({ preventScroll: true })
    } else if (overviewScrollTop.current !== undefined) {
      customerFilterInput.current?.focus({ preventScroll: true })
      if (props.scrollContainer.current) {
        props.scrollContainer.current.scrollTop = overviewScrollTop.current
      }
      overviewScrollTop.current = undefined
    }
  }, [selectedUser, props.scrollContainer])

  if (selectedUser) {
    const customerName = selectedUser.display_name || selectedUser.username
    const customerReport = detail.data?.data
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
          />
        )}
      </section>
    )
  }

  return (
    <div className='flex flex-col gap-4'>
      <section aria-labelledby={overviewHeadingId} className='space-y-3'>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <h2 id={overviewHeadingId} className='text-base font-semibold'>
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
        <UsageReportStatus report={globalReport} showReportPeriod />
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
            </>
          )}
        {globalReport &&
          (globalReport.published_days?.length ?? 1) > 0 &&
          !globalReport.result.customers.length && (
            <UsageReportEmpty period={globalReport.period} />
          )}
      </section>
    </div>
  )
}
