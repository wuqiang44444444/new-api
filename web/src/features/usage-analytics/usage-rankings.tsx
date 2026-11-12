import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ResultPagination } from '@/components/data-table/core/result-pagination'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { ReportCard } from '@/components/report-card'
import { Button } from '@/components/ui/button'
import {
  billingModeLabel,
  formatCustomerStatementQuota,
} from '@/features/billing-reconciliation/lib'
import { cn } from '@/lib/utils'

import {
  getAdminUsageRanking,
  type UsageRankMetric,
  type UsageReportQuery,
} from './api'
import { UsageNonTokenCell, UsageTokensCell } from './metrics-cells'
import {
  PersonalUsageModelCharts,
  type UsageModelRank,
} from './personal-usage-model-charts'
import type {
  UsageAnalyticsMetrics,
  UsageCustomerView,
  UsageReportRankItem,
  UsageUpstreamUrlGroup,
} from './types'
import { formatNumber, mergeUsageNonToken } from './utils'

function accumulate(
  groups: Map<string, UsageModelRank>,
  id: string,
  label: string,
  metrics: UsageAnalyticsMetrics,
  upstream = false
) {
  const row = groups.get(id) ?? {
    id,
    label,
    calls: 0,
    failureCalls: 0,
    tokens: 0,
    missingTokens: false,
    partialAmount: false,
    nonToken: {},
  }
  row.calls += metrics.total_calls ?? 0
  row.failureCalls = (row.failureCalls ?? 0) + metrics.failure_calls
  row.tokens += (metrics.input_tokens ?? 0) + (metrics.output_tokens ?? 0)
  row.missingTokens ||= (metrics.rows_missing_tokens ?? 0) > 0
  const amount = upstream
    ? (metrics.reference_amount ?? metrics.known_reference_amount)
    : metrics.net_quota
  if (amount != null) row.amount = (row.amount ?? 0) + amount
  row.partialAmount ||= upstream
    ? !!metrics.reference_pending || metrics.known_reference_amount != null
    : amount == null ||
      (metrics.rows_missing_money ?? 0) > 0 ||
      (metrics.rows_money_pending ?? 0) > 0
  row.nonToken ??= {}
  mergeUsageNonToken(row.nonToken, metrics)
  groups.set(id, row)
}

export function UsageUpstreamModelCharts(props: {
  groups: UsageUpstreamUrlGroup[]
}) {
  const { t } = useTranslation()
  const rows = new Map<string, UsageModelRank>()
  for (const group of props.groups) {
    for (const model of group.models) {
      const name = model.provider_model
      const fallback = name !== 'unknown' && !!model.provider_model_fallback
      const id = JSON.stringify([name, fallback])
      let label = name
      if (fallback) {
        label = `${name} · ${t('Provider model unrecorded')}`
      }
      accumulate(rows, id, label, model.total, true)
    }
  }
  const models = [...rows.values()]
  return (
    <>
      <PersonalUsageModelCharts rows={models} upstream />
      <Ranking title={t('Model usage details')} rows={models} upstream />
    </>
  )
}

export function UsageModelRanking({ view }: { view: UsageCustomerView }) {
  const { t } = useTranslation()
  const rows = useMemo(() => {
    const groups = new Map<string, UsageModelRank>()
    for (const key of view.keys) {
      for (const model of key.models) {
        accumulate(groups, model.model_name, model.model_name, model.total)
      }
    }
    return [...groups.values()]
  }, [view.keys])
  return (
    <>
      <PersonalUsageModelCharts rows={rows} />
      <Ranking title={t('Model usage details')} rows={rows} />
    </>
  )
}

function publishedRankRow(
  row: UsageReportRankItem,
  upstream: boolean
): UsageModelRank {
  const billingMode = upstream ? undefined : (row.billing_mode ?? 'unknown')
  return {
    id: upstream
      ? String(row.channel_id ?? 0)
      : JSON.stringify([row.name, billingMode]),
    label: upstream ? `${row.name} · #${row.channel_id ?? 0}` : row.name,
    billingMode,
    calls: row.calls,
    failureCalls: row.failure_calls,
    tokens: row.tokens,
    missingTokens: row.missing_tokens > 0,
    missingTokenRows:
      billingMode === 'unknown' ? row.missing_tokens : undefined,
    tokenUsage: row.token_usage && {
      ...row.token_usage,
      rows_missing_tokens: row.missing_tokens,
    },
    amount: row.amount ?? undefined,
    partialAmount: row.pending_amount > 0,
    nonToken: row.non_token,
  }
}

export function AdminCustomerModelRanking(props: {
  rows: UsageReportRankItem[]
}) {
  const { t } = useTranslation()
  return (
    <Ranking
      title={t('Model ranking')}
      rows={props.rows.map((row) => publishedRankRow(row, false))}
      showBillingMode
    />
  )
}

export function PublishedUsageRanking(props: {
  kind: 'model' | 'channel'
  params: UsageReportQuery & { url_key?: string }
  onChannelSelect?: (
    channel: { channel_id: number; channel_name: string },
    button: HTMLButtonElement
  ) => void
}) {
  const { t } = useTranslation()
  const [metric, setMetric] = useState<UsageRankMetric>('amount')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const query = useQuery({
    placeholderData: keepPreviousData,
    queryKey: [
      'published-usage-ranking',
      props.kind,
      props.params,
      metric,
      page,
      pageSize,
    ],
    queryFn: ({ signal }) =>
      getAdminUsageRanking(
        props.kind,
        {
          ...props.params,
          rank_metric: metric,
          rank_page: page,
          rank_page_size: pageSize,
        },
        signal
      ),
  })
  const upstream = props.kind === 'channel'
  const items = query.data?.items ?? []
  return (
    <Ranking
      title={t(upstream ? 'Channel ranking' : 'Model ranking')}
      upstream={upstream}
      rows={items.map((row) => publishedRankRow(row, upstream))}
      showBillingMode={!upstream}
      onSelect={
        props.onChannelSelect &&
        ((id, button) => {
          const row = items.find((item) => String(item.channel_id ?? 0) === id)
          if (row) {
            props.onChannelSelect?.(
              {
                channel_id: row.channel_id ?? 0,
                channel_name: row.name,
              },
              button
            )
          }
        })
      }
      published={{
        metric,
        setMetric: (next) => {
          setMetric(next)
          setPage(1)
        },
        page,
        pageSize,
        total: query.data?.total,
        onPageChange: (nextPage, nextSize) => {
          setPage(nextPage)
          setPageSize(nextSize)
        },
        loading: query.isPending || query.isPlaceholderData,
        fetching: query.isFetching,
        retry: query.isError ? () => void query.refetch() : undefined,
      }}
    />
  )
}

function Ranking({
  title,
  rows,
  upstream = false,
  onSelect,
  published,
  showBillingMode = false,
}: {
  title?: string
  rows: UsageModelRank[]
  upstream?: boolean
  onSelect?: (value: string, button: HTMLButtonElement) => void
  showBillingMode?: boolean
  published?: {
    metric: UsageRankMetric
    setMetric: (metric: UsageRankMetric) => void
    page: number
    pageSize: number
    total?: number
    onPageChange: (page: number, size: number) => void
    loading: boolean
    fetching: boolean
    retry?: () => void
  }
}) {
  const { t } = useTranslation()
  const [localMetric, setLocalMetric] = useState<UsageRankMetric>('amount')
  const metric = published?.metric ?? localMetric
  const setMetric = published?.setMetric ?? setLocalMetric
  const remote = !!published
  const channelRanking = upstream && remote
  const [localPage, setLocalPage] = useState(1)
  const [localPageSize, setLocalPageSize] = useState(10)
  const sorted = useMemo(
    () =>
      remote
        ? rows
        : [...rows].sort(
            (a, b) =>
              (b[metric] ?? -Infinity) - (a[metric] ?? -Infinity) ||
              a.label.localeCompare(b.label) ||
              a.id.localeCompare(b.id)
          ),
    [metric, remote, rows]
  )
  const page = Math.min(
    localPage,
    Math.max(1, Math.ceil(sorted.length / localPageSize))
  )
  const pageRows = remote
    ? sorted
    : sorted.slice((page - 1) * localPageSize, page * localPageSize)
  const barMax = sorted[0]?.[metric] ?? 0
  const showBars =
    barMax > 0 &&
    sorted.every((row) => {
      if (metric === 'amount') {
        return row.amount != null && row.amount >= 0 && !row.partialAmount
      }
      return metric !== 'tokens' || !row.missingTokens
    })
  if (!remote && rows.length === 0) return null
  return (
    <ReportCard>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        {title && <h2 className='text-sm font-medium'>{title}</h2>}
        <div className='ml-auto flex flex-wrap gap-1'>
          {(
            [
              ['calls', t('Calls')],
              ['tokens', t('Total tokens')],
              ['amount', t(upstream ? 'Reference amount' : 'Net amount')],
            ] as const
          ).map(([value, label]) => (
            <Button
              key={value}
              variant={metric === value ? 'secondary' : 'ghost'}
              size='sm'
              aria-pressed={metric === value}
              onClick={() => {
                setMetric(value)
                setLocalPage(1)
              }}
            >
              {label}
            </Button>
          ))}
        </div>
      </div>
      {published?.loading && <LoadingState />}
      {published?.retry && <ErrorState onRetry={published.retry} />}
      <div
        className='relative overflow-x-auto'
        hidden={published?.loading || !!published?.retry}
      >
        <table
          aria-label={title ?? t('Model usage details')}
          className='w-full min-w-[34rem] text-sm'
        >
          <thead>
            <tr className='border-b text-left'>
              <th scope='col' className='py-2'>
                {t(channelRanking ? 'Channel' : 'Model')}
              </th>
              {showBillingMode && (
                <th scope='col' className='px-2'>
                  {t('Billing mode')}
                </th>
              )}
              <th scope='col' className='px-2 text-right'>
                {t('Calls')}
              </th>
              {!channelRanking && (
                <th scope='col' className='px-2 text-right whitespace-nowrap'>
                  {t('Failed calls')}
                </th>
              )}
              <th scope='col' className='px-2 text-right'>
                {t('Total tokens')}
              </th>
              <th scope='col' className='px-2 text-right'>
                {t('Non-Token usage')}
              </th>
              <th scope='col' className='px-2 text-right'>
                {t(upstream ? 'Reference amount' : 'Net amount')}
              </th>
            </tr>
          </thead>
          <tbody>
            {pageRows.map((row) => {
              let modeLabel: string | undefined
              if (showBillingMode) {
                modeLabel =
                  row.billingMode === 'per_image'
                    ? t('Per image')
                    : t(billingModeLabel(row.billingMode ?? 'unknown'))
              }
              const tokenApplicable =
                !showBillingMode || row.billingMode === 'token'
              return (
                <tr key={row.id} className='border-b'>
                  <td className='py-2'>
                    {onSelect ? (
                      <Button
                        variant='link'
                        size='sm'
                        className='h-auto px-0 text-left whitespace-normal'
                        onClick={(event) =>
                          onSelect(row.id, event.currentTarget)
                        }
                      >
                        {row.label}
                      </Button>
                    ) : (
                      row.label
                    )}
                    {showBars && (
                      <div
                        className='bg-muted mt-1 h-1 w-24 rounded'
                        aria-hidden='true'
                      >
                        <div
                          className='bg-primary h-full rounded'
                          style={{
                            width: `${((row[metric] ?? 0) / barMax) * 100}%`,
                          }}
                        />
                      </div>
                    )}
                  </td>
                  {showBillingMode && <td className='px-2'>{modeLabel}</td>}
                  <td className='px-2 text-right tabular-nums'>
                    {formatNumber(row.calls)}
                  </td>
                  {!channelRanking && (
                    <td
                      className={cn(
                        'px-2 text-right tabular-nums',
                        (row.failureCalls ?? 0) > 0 && 'text-destructive'
                      )}
                    >
                      {row.failureCalls == null
                        ? t('unrecorded')
                        : formatNumber(row.failureCalls)}
                    </td>
                  )}
                  <td className='px-2 py-2 text-right tabular-nums'>
                    {!tokenApplicable &&
                      (row.missingTokenRows ? (
                        <span className='text-muted-foreground text-xs'>
                          {t('{{count}} rows unrecorded', {
                            count: row.missingTokenRows,
                          })}
                        </span>
                      ) : (
                        <>
                          <span aria-hidden='true'>—</span>
                          <span className='sr-only'>{t('Not applicable')}</span>
                        </>
                      ))}
                    {tokenApplicable && row.tokenUsage && (
                      <UsageTokensCell metrics={row.tokenUsage} />
                    )}
                    {tokenApplicable &&
                      !row.tokenUsage &&
                      (row.missingTokens && row.tokens === 0
                        ? t('unrecorded')
                        : formatNumber(row.tokens))}
                  </td>
                  <td className='px-2 text-right'>
                    <UsageNonTokenCell metrics={row.nonToken} />
                  </td>
                  <td className='px-2 text-right tabular-nums'>
                    {upstream && row.partialAmount && row.amount == null
                      ? t('Pending reconciliation')
                      : formatCustomerStatementQuota(row.amount)}
                    {!upstream && row.partialAmount && (
                      <span className='text-muted-foreground block text-xs'>
                        {t('Pending')}
                      </span>
                    )}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
      {published ? (
        published.total != null &&
        published.total > 0 && (
          <ResultPagination
            page={published.page}
            pageSize={published.pageSize}
            total={published.total}
            onChange={published.onPageChange}
            disabled={published.fetching}
            label={title ?? t('Model usage details')}
          />
        )
      ) : (
        <ResultPagination
          page={page}
          pageSize={localPageSize}
          total={sorted.length}
          onChange={(nextPage, nextSize) => {
            setLocalPage(nextPage)
            setLocalPageSize(nextSize)
          }}
          label={title ?? t('Model usage details')}
        />
      )}
    </ReportCard>
  )
}
