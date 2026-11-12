import { useQuery } from '@tanstack/react-query'
import { type RefObject, useId, useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'

import { getAdminUsageUpstreamSummary } from './api'
import { UsageExportButton } from './export-button'
import type { UsagePeriodKind, UsageUpstreamChannelTotal } from './types'
import { UpstreamUsageReport } from './upstream-usage-report'
import { PublishedUsageRanking } from './usage-rankings'
import { UsageReportEmpty } from './usage-report-empty'
import { UsageReportStatus } from './usage-report-status'
import { usageReportDayQuery } from './utils'

export function AdminUpstreamUsage(props: {
  period: UsagePeriodKind
  date: string
  reportDays?: string
  scrollContainer: RefObject<HTMLDivElement | null>
  onDaySelect: (
    day: string,
    reportDays?: string,
    periodReportDays?: string
  ) => void
  returnToPeriod?: {
    period: Exclude<UsagePeriodKind, 'day'>
    onClick: () => void
  }
}) {
  const { t } = useTranslation()
  const channelFilterId = useId()
  const heading = useRef<HTMLHeadingElement>(null)
  const returnTo = useRef<{ button?: HTMLButtonElement; scroll: number }>(null)
  const [url, setURL] = useState<{ value: string; label: string }>()
  const [channel, setChannel] = useState<{
    channel_id: number
    channel_name: string
  }>()
  const urlKey = url?.value
  const channelID = channel?.channel_id
  const key = [
    'usage-admin-upstream',
    props.period,
    props.date,
    props.reportDays,
  ]
  const params = {
    period: props.period,
    date: props.date,
    report_days: props.reportDays,
  }
  const overview = useQuery({
    queryKey: key,
    queryFn: ({ signal }) =>
      getAdminUsageUpstreamSummary({ ...params, compact: true }, signal),
  })
  const versions = overview.data?.data.report_days
  const reportDays = versions?.length
    ? JSON.stringify(versions)
    : props.reportDays
  const overviewPublished =
    overview.isSuccess && (overview.data.data.published_days?.length ?? 1) > 0
  const urlReport = useQuery({
    queryKey: [...key, reportDays, urlKey],
    enabled: !!urlKey && overviewPublished,
    queryFn: ({ signal }) =>
      getAdminUsageUpstreamSummary(
        { ...params, report_days: reportDays, url_key: urlKey, compact: true },
        signal
      ),
  })
  const detail = useQuery({
    queryKey: [...key, reportDays, urlKey, channelID],
    enabled: channelID !== undefined && overviewPublished,
    queryFn: ({ signal }) =>
      getAdminUsageUpstreamSummary(
        {
          ...params,
          report_days: reportDays,
          url_key: urlKey,
          channel_id: channelID,
        },
        signal
      ),
  })

  const list = urlKey ? urlReport : overview
  let active = overview
  if (overviewPublished) active = channel ? detail : list
  const report = active.data?.data
  const hasPublishedDays = !!report && (report.published_days?.length ?? 1) > 0
  const hasUsage = !!report?.result.url_groups.length
  const channels = list.data?.data.result.channels ?? EMPTY_CHANNELS
  const urlOptions = (overview.data?.data.result.url_groups ?? []).map(
    (group) => ({
      value: group.url_key,
      label: group.display_name,
    })
  )
  if (url && !urlOptions.some((option) => option.value === url.value)) {
    urlOptions.push(url)
  }
  const channelOptions = channels.map((row) => ({
    value: String(row.channel_id),
    label: `${row.channel_name} · #${row.channel_id}`,
  }))
  const channelName =
    report?.result.channels?.find((row) => row.channel_id === channelID)
      ?.channel_name ?? channel?.channel_name
  const channelLabel = channel
    ? `${channelName} · #${channel.channel_id}`
    : undefined
  if (
    channel &&
    channelLabel &&
    !channelOptions.some(
      (option) => option.value === String(channel.channel_id)
    )
  ) {
    channelOptions.push({
      value: String(channel.channel_id),
      label: channelLabel,
    })
  }

  function openChannel(
    row: NonNullable<typeof channel>,
    button?: HTMLButtonElement
  ) {
    if (!channel) {
      returnTo.current = {
        button,
        scroll: props.scrollContainer.current?.scrollTop ?? 0,
      }
    }
    setChannel(row)
  }

  useLayoutEffect(() => {
    if (channelID !== undefined) {
      if (props.scrollContainer.current) {
        props.scrollContainer.current.scrollTop = 0
      }
      heading.current?.focus({ preventScroll: true })
    } else if (returnTo.current) {
      const target = returnTo.current
      returnTo.current = null
      const control = target.button?.isConnected
        ? target.button
        : props.scrollContainer.current?.querySelector<HTMLInputElement>(
            `[id="${channelFilterId}"]`
          )
      control?.focus({ preventScroll: true })
      if (props.scrollContainer.current) {
        props.scrollContainer.current.scrollTop = target.scroll
      }
    }
  }, [channelID, channelFilterId, props.scrollContainer])

  return (
    <section
      className='flex min-w-0 flex-col gap-4'
      aria-labelledby={`${channelFilterId}-heading`}
    >
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <div className='flex min-w-0 flex-wrap items-center gap-3'>
          {props.returnToPeriod && (
            <Button
              variant='outline'
              size='sm'
              onClick={props.returnToPeriod.onClick}
            >
              {props.returnToPeriod.period === 'month'
                ? t('Back to monthly summary')
                : t('Back to weekly summary')}
            </Button>
          )}
          {channel && (
            <Button
              variant='outline'
              size='sm'
              onClick={() => setChannel(undefined)}
            >
              {t('Back to channel ranking')}
            </Button>
          )}
          <h2
            id={`${channelFilterId}-heading`}
            ref={heading}
            tabIndex={-1}
            className='text-base font-semibold wrap-anywhere'
          >
            {channelLabel ?? t('Upstream usage overview')}
          </h2>
        </div>
        <div className='flex min-w-0 flex-wrap items-center gap-2'>
          <Combobox
            aria-label={t('Upstream URL')}
            className='w-60 max-w-full'
            value={urlKey ?? ''}
            options={[
              { value: '', label: t('All upstream URLs') },
              ...urlOptions,
            ]}
            onValueChange={(value) => {
              setURL(urlOptions.find((option) => option.value === value))
              returnTo.current = null
              setChannel(undefined)
            }}
          />
          <Combobox
            id={channelFilterId}
            aria-label={t('Channel')}
            className='w-60 max-w-full'
            value={channelID === undefined ? '' : String(channelID)}
            options={[
              { value: '', label: t('All channels') },
              ...channelOptions,
            ]}
            onValueChange={(value) => {
              const row = channels.find(
                (item) => String(item.channel_id) === value
              )
              if (row) openChannel(row)
              else if (!value) setChannel(undefined)
            }}
          />
          <UsageExportButton
            body={{
              period: props.period,
              date: props.date,
              view: 'upstream',
              url_key: urlKey,
              channel_id: channelID,
              report_days: report?.report_days,
            }}
            period={report?.period}
            upstreamURLName={url?.label}
            upstreamChannelName={channelLabel}
            snapshotAt={report?.generated_at}
            ready={
              active.isSuccess &&
              !active.isFetching &&
              hasPublishedDays &&
              hasUsage
            }
          />
        </div>
      </div>
      {report && (
        <UsageReportStatus
          report={{
            ...report,
            incomplete_days:
              overview.data?.data.incomplete_days ?? report.incomplete_days,
          }}
          showReportPeriod
        />
      )}
      {(overview.isError || active.isError) && (
        <ErrorState
          onRetry={() => void (overview.isError ? overview : active).refetch()}
        />
      )}
      {!overview.isError && !active.isError && !report && <LoadingState />}
      {report && !hasPublishedDays && (
        <EmptyState
          bordered
          title={t('Some usage data is still being prepared')}
        />
      )}
      {report && hasPublishedDays && !hasUsage && (
        <UsageReportEmpty period={report.period} />
      )}
      {report && hasPublishedDays && (hasUsage || props.period !== 'day') && (
        <UpstreamUsageReport
          report={{
            ...report,
            incomplete_days:
              overview.data?.data.incomplete_days ?? report.incomplete_days,
          }}
          showModels={!!channel}
          onDaySelect={(day) =>
            props.onDaySelect(day, usageReportDayQuery(report, day), reportDays)
          }
        />
      )}
      <div hidden={!!channel || channels.length === 0}>
        {channels.length > 0 && (
          <PublishedUsageRanking
            key={`${props.period}:${props.date}:${reportDays ?? ''}:${urlKey ?? ''}`}
            kind='channel'
            params={{ ...params, report_days: reportDays, url_key: urlKey }}
            onChannelSelect={openChannel}
          />
        )}
      </div>
    </section>
  )
}

const EMPTY_CHANNELS: UsageUpstreamChannelTotal[] = []
