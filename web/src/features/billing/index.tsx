/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { AlertTriangle, Info, ReceiptText, RefreshCw } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import {
  Alert,
  AlertAction,
  AlertDescription,
  AlertTitle,
} from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import dayjs from '@/lib/dayjs'
import { formatTimestampToDate } from '@/lib/format'

import { getBillingBreakdown, getBillingStatement } from './api'
import { BillingBreakdown } from './components/billing-breakdown'
import { BillingFilters } from './components/billing-filters'
import { BillingStatementTable } from './components/billing-statement-table'
import { BillingSummaryCards } from './components/billing-summary'
import {
  resolveBillingPeriod,
  summarizeBillingBreakdownItems,
  summarizeBillingItems,
} from './lib'
import {
  countBillingBreakdownMismatches,
  selectReliableBillingBreakdownItems,
} from './lib/statement-rows'
import type { BillingPreset, BillingSearch, BillingView } from './types'

type BillingProps = {
  search: BillingSearch
}

export function Billing(props: BillingProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const preset = props.search.preset ?? 'this-week'
  const view = props.search.view ?? 'detail'
  const period = useMemo(
    () =>
      resolveBillingPeriod(
        preset,
        props.search.startTime,
        props.search.endTime
      ),
    [preset, props.search.endTime, props.search.startTime]
  )
  const billingQuery = useQuery({
    queryKey: [
      'billing-statement',
      period.start_timestamp,
      period.end_timestamp,
    ],
    queryFn: async () => {
      const response = await getBillingStatement(
        period.start_timestamp,
        period.end_timestamp
      )
      if (!response.success || !response.data) {
        throw new Error(
          response.message || t('We could not load the usage statement.')
        )
      }
      return response.data
    },
    staleTime: 60 * 1000,
    retry: false,
  })
  const breakdownQuery = useQuery({
    queryKey: [
      'billing-statement-breakdown',
      period.start_timestamp,
      period.end_timestamp,
    ],
    queryFn: async () => {
      const response = await getBillingBreakdown(
        period.start_timestamp,
        period.end_timestamp
      )
      if (!response.success || !response.data) {
        throw new Error(response.message || 'Unable to load usage breakdown')
      }
      return response.data
    },
    enabled: billingQuery.isSuccess,
    staleTime: 60 * 1000,
    retry: false,
  })

  const statement = billingQuery.data
  const keyOptions = useMemo(() => {
    const options = new Map<number, string>()
    for (const item of statement?.items ?? []) {
      if (item.token_id <= 0) continue
      options.set(
        item.token_id,
        item.token_name ||
          t('Deleted API Key ({{id}})', {
            id: item.token_id,
          })
      )
    }
    return [...options.entries()]
      .sort((left, right) => left[1].localeCompare(right[1]))
      .map(([value, label]) => ({ value: String(value), label }))
  }, [statement?.items, t])
  const modelOptions = useMemo(
    () =>
      [...new Set((statement?.items ?? []).map((item) => item.model_name))]
        .filter(Boolean)
        .sort()
        .map((model) => ({ value: model, label: model })),
    [statement?.items]
  )
  const filteredItems = useMemo(
    () =>
      (statement?.items ?? []).filter(
        (item) =>
          (props.search.tokenId == null ||
            item.token_id === props.search.tokenId) &&
          (!props.search.model || item.model_name === props.search.model)
      ),
    [props.search.model, props.search.tokenId, statement?.items]
  )
  const filteredSummary = useMemo(
    () => summarizeBillingItems(filteredItems, period),
    [filteredItems, period]
  )
  const filteredBreakdownItems = useMemo(
    () =>
      (breakdownQuery.data?.items ?? []).filter(
        (item) =>
          (props.search.tokenId == null ||
            item.token_id === props.search.tokenId) &&
          (!props.search.model || item.model_name === props.search.model)
      ),
    [breakdownQuery.data?.items, props.search.model, props.search.tokenId]
  )
  const reliableBreakdownItems = useMemo(
    () =>
      selectReliableBillingBreakdownItems(
        filteredItems,
        filteredBreakdownItems
      ),
    [filteredBreakdownItems, filteredItems]
  )
  const filteredBreakdownSummary = useMemo(
    () => summarizeBillingBreakdownItems(reliableBreakdownItems),
    [reliableBreakdownItems]
  )
  const breakdownMismatches = useMemo(
    () =>
      countBillingBreakdownMismatches(filteredItems, filteredBreakdownItems),
    [filteredBreakdownItems, filteredItems]
  )

  const updateSearch = (patch: Partial<BillingSearch>) => {
    navigate({
      to: '/billing',
      search: { ...props.search, ...patch },
    })
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        <span className='inline-flex min-w-0 items-center gap-2'>
          <ReceiptText className='text-primary size-5 shrink-0' />
          <span className='truncate'>{t('Usage Statement')}</span>
          <Badge variant='outline' className='shrink-0'>
            {preset === 'this-week' ? t('This Week') : t('Selected Period')}
          </Badge>
        </span>
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='space-y-3'>
          <div>
            <p className='text-muted-foreground text-sm'>
              {t(
                'Review token usage, call performance, and currently settled costs by API key and model.'
              )}
            </p>
          </div>

          <BillingFilters
            preset={preset}
            period={period}
            tokenId={props.search.tokenId}
            model={props.search.model}
            keyOptions={keyOptions}
            modelOptions={modelOptions}
            onPresetChange={(nextPreset: BillingPreset) =>
              updateSearch({
                preset: nextPreset,
                startTime: undefined,
                endTime: undefined,
              })
            }
            onPeriodChange={(startTime, endTime) =>
              updateSearch({
                preset: 'custom',
                startTime,
                endTime,
              })
            }
            onTokenChange={(tokenId) => updateSearch({ tokenId })}
            onModelChange={(model) => updateSearch({ model })}
          />

          {billingQuery.isError ? (
            <ErrorState
              title={t('Unable to load usage statement')}
              description={
                billingQuery.error instanceof Error
                  ? billingQuery.error.message
                  : t('Please try again later.')
              }
              onRetry={() => billingQuery.refetch()}
            />
          ) : (
            <>
              <BillingSummaryCards
                summary={filteredSummary}
                funds={statement?.funds}
                loading={billingQuery.isLoading}
              />
              <BillingBreakdown summary={filteredBreakdownSummary} />
              {breakdownQuery.isError && (
                <Alert variant='destructive'>
                  <AlertTriangle />
                  <AlertTitle>{t('Usage composition unavailable')}</AlertTitle>
                  <AlertDescription>
                    {t(
                      'Settled costs and base tokens remain available. Retry to load cache and Context details.'
                    )}
                  </AlertDescription>
                  <AlertAction>
                    <Button
                      variant='outline'
                      size='sm'
                      onClick={() => breakdownQuery.refetch()}
                    >
                      <RefreshCw />
                      {t('Retry')}
                    </Button>
                  </AlertAction>
                </Alert>
              )}
              {!breakdownQuery.isLoading &&
                !breakdownQuery.isError &&
                breakdownMismatches > 0 && (
                  <Alert>
                    <AlertTriangle />
                    <AlertTitle>
                      {t('Usage composition unavailable or changed')}
                    </AlertTitle>
                    <AlertDescription>
                      {t(
                        'Some usage details are unavailable or no longer match the settled statement snapshot. Refresh before exporting.'
                      )}
                    </AlertDescription>
                    <AlertAction>
                      <Button
                        variant='outline'
                        size='sm'
                        onClick={async () => {
                          await Promise.all([
                            billingQuery.refetch(),
                            breakdownQuery.refetch(),
                          ])
                        }}
                      >
                        <RefreshCw />
                        {t('Refresh')}
                      </Button>
                    </AlertAction>
                  </Alert>
                )}
              <BillingStatementTable
                items={filteredItems}
                breakdownItems={filteredBreakdownItems}
                breakdownLoading={breakdownQuery.isLoading}
                breakdownUnavailable={breakdownQuery.isError}
                period={period}
                view={view}
                loading={billingQuery.isLoading}
                onViewChange={(nextView: BillingView) =>
                  updateSearch({ view: nextView })
                }
              />
              <Alert>
                <Info />
                <AlertTitle>{t('Statement basis')}</AlertTitle>
                <AlertDescription>
                  {t(
                    'Costs come from settled consume logs and are not recalculated with current prices. Open usage details to review the calculation for an individual call.'
                  )}{' '}
                  {statement != null &&
                    t(
                      'Range: {{start}} to {{end}} · Time zone: {{timezone}} · Generated: {{generated}}',
                      {
                        start: formatTimestampToDate(
                          statement.period.start_timestamp
                        ),
                        end: formatTimestampToDate(
                          statement.period.end_timestamp
                        ),
                        timezone:
                          Intl.DateTimeFormat().resolvedOptions().timeZone ||
                          dayjs().format('Z'),
                        generated: formatTimestampToDate(
                          statement.generated_at
                        ),
                      }
                    )}{' '}
                  {t(
                    'This is a real-time aggregate, not a frozen monthly statement; late settlements, refunds, or adjustments may change it.'
                  )}
                </AlertDescription>
              </Alert>
            </>
          )}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
