import { useQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { formatCustomerStatementQuota } from '@/features/billing-reconciliation/lib'

import {
  getAdminUsageRanking,
  type UsageRankMetric,
  type UsageReportQuery,
} from './api'
import {
  UsageCallsCell,
  UsageNonTokenCell,
  UsageTokensCell,
} from './metrics-cells'
import {
  PersonalUsageModelCharts,
  type UsageModelRank,
} from './personal-usage-model-charts'
import type {
  UsageAnalyticsMetrics,
  UsageCustomerView,
  UsageUpstreamUrlGroup,
} from './types'
import { formatNumber, mergeUsageNonToken, usageNonTokenValues } from './utils'

export function UsageOverview({ metrics }: { metrics: UsageAnalyticsMetrics }) {
  const { t } = useTranslation()
  const missing = (metrics.rows_missing_tokens ?? 0) > 0
  const tokens = (metrics.input_tokens ?? 0) + (metrics.output_tokens ?? 0)
  const amount = metrics.net_quota
  return (
    <div className='grid gap-3 sm:grid-cols-[repeat(auto-fit,minmax(10rem,1fr))]'>
      <section className='space-y-2 rounded-md border p-3'>
        <h2 className='font-medium'>{t('Calls')}</h2>
        <UsageCallsCell metrics={metrics} />
      </section>
      <section className='space-y-2 rounded-md border p-3'>
        <h2 className='font-medium'>{t('Total tokens')}</h2>
        <p className='tabular-nums'>
          {missing && tokens === 0 ? t('unrecorded') : formatNumber(tokens)}
        </p>
        <details>
          <summary className='cursor-pointer text-sm'>
            {t('View token details')}
          </summary>
          <UsageTokensCell metrics={metrics} />
        </details>
      </section>
      {usageNonTokenValues(t, metrics).length > 0 && (
        <section className='space-y-2 rounded-md border p-3'>
          <h2 className='font-medium'>{t('Non-Token usage')}</h2>
          <UsageNonTokenCell metrics={metrics} />
        </section>
      )}
      <section className='space-y-2 rounded-md border p-3'>
        <h2 className='font-medium'>{t('Net amount')}</h2>
        <p className='tabular-nums'>{formatCustomerStatementQuota(amount)}</p>
        {(metrics.rows_money_pending ?? 0) > 0 && (
          <p className='text-muted-foreground text-sm'>{t('Pending')}</p>
        )}
      </section>
    </div>
  )
}

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
    tokens: 0,
    missingTokens: false,
    partialAmount: false,
    nonToken: {},
  }
  row.calls += metrics.total_calls ?? 0
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
      <UsageModelDetails rows={models} upstream />
    </>
  )
}

function UsageModelDetails(props: {
  rows: UsageModelRank[]
  upstream?: boolean
}) {
  const { t } = useTranslation()
  if (!props.rows.length) return null
  return (
    <Collapsible className='space-y-2'>
      <CollapsibleTrigger render={<Button size='sm' variant='outline' />}>
        {t('Model usage details')}
      </CollapsibleTrigger>
      <CollapsibleContent>
        <Ranking rows={props.rows} upstream={props.upstream} />
      </CollapsibleContent>
    </Collapsible>
  )
}

export function UsageModelRanking({
  view,
  selected,
  onSelect,
  showDetails,
}: {
  view: UsageCustomerView
  selected?: string
  showDetails?: boolean
  onSelect?: (value: string | undefined) => void
}) {
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
  if (!onSelect) {
    return (
      <>
        <PersonalUsageModelCharts rows={rows} />
        {showDetails && <UsageModelDetails rows={rows} />}
      </>
    )
  }
  return (
    <Ranking
      title={t('Model ranking')}
      rows={rows}
      selected={selected}
      onSelect={onSelect}
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
  const query = useQuery({
    queryKey: [
      'published-usage-ranking',
      props.kind,
      props.params,
      metric,
      page,
    ],
    queryFn: ({ signal }) =>
      getAdminUsageRanking(
        props.kind,
        {
          ...props.params,
          rank_metric: metric,
          rank_page: page,
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
      rows={items.map((row) => ({
        id: upstream ? String(row.channel_id ?? 0) : row.name,
        label: upstream ? `${row.name} · #${row.channel_id ?? 0}` : row.name,
        calls: row.calls,
        tokens: row.tokens,
        missingTokens: row.missing_tokens > 0,
        amount: row.amount ?? undefined,
        partialAmount: row.pending_amount > 0,
        nonToken: row.non_token,
      }))}
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
        setPage,
        hasMore: !!query.data?.has_more,
        loading: query.isPending,
        retry: query.isError ? () => void query.refetch() : undefined,
      }}
    />
  )
}

function Ranking({
  title,
  rows,
  upstream = false,
  selected,
  onSelect,
  published,
}: {
  title?: string
  rows: UsageModelRank[]
  upstream?: boolean
  selected?: string
  onSelect?: (value: string | undefined, button: HTMLButtonElement) => void
  published?: {
    metric: UsageRankMetric
    setMetric: (metric: UsageRankMetric) => void
    page: number
    setPage: (page: number) => void
    hasMore: boolean
    loading: boolean
    retry?: () => void
  }
}) {
  const { t } = useTranslation()
  const [localMetric, setLocalMetric] = useState<UsageRankMetric>('amount')
  const metric = published?.metric ?? localMetric
  const setMetric = published?.setMetric ?? setLocalMetric
  const remote = !!published
  const [visible, setVisible] = useState(10)
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
    <section className='space-y-2 rounded-md border p-3'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        {title && <h2 className='font-medium'>{title}</h2>}
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
              onClick={() => setMetric(value)}
            >
              {label}
            </Button>
          ))}
        </div>
      </div>
      {published?.loading && <LoadingState />}
      {published?.retry && <ErrorState onRetry={published.retry} />}
      <div
        className='overflow-x-auto'
        hidden={published?.loading || !!published?.retry}
      >
        <table
          aria-label={title ?? t('Model usage details')}
          className='w-full min-w-[34rem] text-sm'
        >
          <thead>
            <tr className='border-b text-left'>
              <th scope='col' className='py-2'>
                {t(upstream && published ? 'Channel' : 'Model')}
              </th>
              <th scope='col' className='px-2 text-right'>
                {t('Calls')}
              </th>
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
            {(remote ? sorted : sorted.slice(0, visible)).map((row) => (
              <tr key={row.id} className='border-b'>
                <td className='py-2'>
                  {onSelect ? (
                    <Button
                      variant='link'
                      size='sm'
                      className='h-auto px-0 text-left whitespace-normal'
                      aria-pressed={upstream ? undefined : selected === row.id}
                      onClick={(event) =>
                        onSelect(
                          selected === row.id ? undefined : row.id,
                          event.currentTarget
                        )
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
                <td className='px-2 text-right tabular-nums'>
                  {formatNumber(row.calls)}
                </td>
                <td className='px-2 text-right tabular-nums'>
                  {row.missingTokens && row.tokens === 0
                    ? t('unrecorded')
                    : formatNumber(row.tokens)}
                  {row.missingTokens && row.tokens > 0 && (
                    <span className='text-muted-foreground block text-xs'>
                      {t('Known subtotal')}
                    </span>
                  )}
                </td>
                <td className='px-2 text-right'>
                  <UsageNonTokenCell metrics={row.nonToken} />
                </td>
                <td className='px-2 text-right tabular-nums'>
                  {upstream && row.partialAmount && row.amount == null
                    ? t('Pending reconciliation')
                    : formatCustomerStatementQuota(row.amount)}
                  {row.partialAmount && (!upstream || row.amount != null) && (
                    <span className='text-muted-foreground block text-xs'>
                      {t(
                        upstream && row.amount != null
                          ? 'Known subtotal'
                          : 'Pending'
                      )}
                    </span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {published && (published.hasMore || published.page > 1) && (
        <div className='flex flex-wrap gap-2'>
          <Button
            variant='outline'
            size='sm'
            disabled={published.loading || published.page <= 1}
            onClick={() => published.setPage(published.page - 1)}
          >
            {t('Previous page')}
          </Button>
          <Button
            variant='outline'
            size='sm'
            disabled={published.loading || !published.hasMore}
            onClick={() => published.setPage(published.page + 1)}
          >
            {t('Next page')}
          </Button>
        </div>
      )}
      {!published && visible < sorted.length && (
        <Button
          size='sm'
          variant='outline'
          onClick={() => setVisible(visible + 10)}
        >
          {t('More')}
        </Button>
      )}
    </section>
  )
}
