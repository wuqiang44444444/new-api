/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { ArrowLeft01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

import type { BillingReportManifest, UpstreamUrlChannelRow } from '../types'
import { useUpstreamPage } from '../upstream-page-query'
import { BillingPagination } from './billing-pagination'
import { BillingReportStatus } from './billing-report-status'
import { UpstreamDataStatus } from './upstream-data-status'
import {
  UpstreamDetailPanel,
  type UpstreamDetailSelection,
} from './upstream-detail-panel'
import { UpstreamDiscountDialog } from './upstream-discount-dialog'
import { UpstreamExportAction } from './upstream-export-action'
import { UpstreamUrlModels } from './upstream-url-models'
import {
  type UpstreamUrlSort,
  UpstreamUrlSummaryTable,
  UpstreamUrlTotals,
} from './upstream-url-summary'

type Props = {
  month: string
  onMonthChange: (month: string) => void
  period: { start_timestamp: number; end_timestamp: number }
}

export function UpstreamReconciliationView(props: Props) {
  return <UpstreamChannelContent key={props.month} {...props} />
}

function UpstreamChannelContent(props: Props) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [searchDraft, setSearchDraft] = useState('')
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [sort, setSort] = useState<UpstreamUrlSort>('reference_amount')
  const [selectedRow, setSelectedRow] = useState<{
    row: UpstreamUrlChannelRow
    report?: BillingReportManifest
  } | null>(null)
  const [discountTarget, setDiscountTarget] = useState<number | null>(null)
  const [selection, setSelection] = useState<UpstreamDetailSelection | null>(
    null
  )
  const query = useUpstreamPage({
    ...props.period,
    level: 'url_channels',
    search,
    sort_by: sort,
    page,
    page_size: pageSize,
  })
  const report = query.data?.billing_report
  const exportPeriod = {
    job_type: 'upstream_details' as const,
    start_timestamp: props.period.start_timestamp,
    end_timestamp: props.period.end_timestamp + 1,
  }
  const result = query.data?.result
  const quality = result?.data_quality
  const rows = result?.url_channels ?? []
  return (
    <div className='flex min-h-0 flex-1 flex-col gap-3'>
      {selectedRow ? (
        <>
          <div className='flex shrink-0 flex-wrap items-center gap-2'>
            <Button
              variant='ghost'
              size='sm'
              onClick={() => setSelectedRow(null)}
            >
              <HugeiconsIcon
                icon={ArrowLeft01Icon}
                strokeWidth={2}
                data-icon='inline-start'
              />
              {t('Back')}
            </Button>
            <div className='min-w-0 wrap-anywhere'>
              <h2 className='font-semibold'>
                {selectedRow.row.channel_name || t('Channel')} · #
                {selectedRow.row.channel_id}
              </h2>
              <p className='text-muted-foreground text-xs'>
                {selectedRow.row.base_url || selectedRow.row.url_name}
              </p>
            </div>
            <span className='text-muted-foreground text-sm'>{props.month}</span>
            <BillingReportStatus report={selectedRow.report} />
            <UpstreamExportAction
              className='ms-auto'
              payload={{
                ...exportPeriod,
                report_version: selectedRow.report?.version,
                url_key: selectedRow.row.url_key,
                channel_id: selectedRow.row.channel_id,
              }}
              scope={[
                `${selectedRow.row.channel_name || t('Channel')} · #${selectedRow.row.channel_id}`,
                selectedRow.row.base_url || selectedRow.row.url_name,
                t('All models'),
              ]}
              available={
                !!selectedRow.report?.version &&
                !!selectedRow.report.published_days
              }
            />
          </div>
          <UpstreamUrlModels
            urlKey={selectedRow.row.url_key}
            channelId={selectedRow.row.channel_id}
            reportVersion={selectedRow.report?.version}
            period={props.period}
            onViewDetails={(selection) =>
              setSelection({
                ...selection,
                reportVersion: selectedRow.report?.version,
                urlKey: selectedRow.report
                  ? selectedRow.row.url_key
                  : undefined,
              })
            }
          />
        </>
      ) : (
        <>
          <div className='flex shrink-0 flex-wrap items-center justify-between gap-2'>
            <div className='flex min-w-0 flex-wrap items-center gap-2 max-sm:w-full'>
              <label className='flex flex-wrap items-center gap-x-2 gap-y-1'>
                <span className='text-muted-foreground text-xs font-medium'>
                  {t('Billing month')}
                </span>
                <Input
                  className='w-36'
                  type='month'
                  value={props.month}
                  onChange={(event) => {
                    if (event.target.value) {
                      props.onMonthChange(event.target.value)
                    }
                  }}
                />
              </label>
              <UpstreamExportAction
                className='max-sm:ms-auto'
                payload={{
                  ...exportPeriod,
                  report_version: report?.version,
                  all_channels: true,
                }}
                scope={[t('All channels for the billing month')]}
                available={
                  query.isSuccess &&
                  !query.isFetching &&
                  !!report?.version &&
                  !!report.published_days
                }
              />
              <Button
                variant='outline'
                size='sm'
                onClick={() => setDiscountTarget(0)}
              >
                {t('Set discount')}
              </Button>
              <form
                className='flex w-full min-w-0 items-center gap-2 sm:w-auto'
                onSubmit={(event) => {
                  event.preventDefault()
                  setSearch(searchDraft.trim())
                  setPage(1)
                }}
              >
                <Input
                  className='min-w-0 flex-1 sm:w-56'
                  aria-label={t('Upstream URL or channel')}
                  placeholder={t('Upstream URL or channel')}
                  value={searchDraft}
                  maxLength={255}
                  onChange={(event) => setSearchDraft(event.target.value)}
                />
                <Button type='submit' variant='outline' size='sm'>
                  {t('Search')}
                </Button>
              </form>
            </div>
            {!query.isPending && !query.isError && (
              <div className='flex min-w-0 flex-wrap items-center gap-2'>
                <BillingReportStatus report={report} />
                <UpstreamDataStatus
                  scope='report'
                  quality={quality}
                  onViewEvidence={
                    search
                      ? undefined
                      : (entry) =>
                          setSelection({
                            reportVersion: report?.version,
                            evidenceFilter: entry.filter,
                            evidenceLabel: entry.text,
                          })
                  }
                />
              </div>
            )}
          </div>
          {query.isPending && <LoadingState />}
          {query.isError && (
            <ErrorState
              title={t('Unable to load upstream reconciliation')}
              onRetry={() => query.refetch()}
            />
          )}
          {!query.isPending && !query.isError && (
            <>
              {result?.totals && result.total > 0 && (
                <UpstreamUrlTotals
                  totals={result.totals}
                  quality={quality}
                  filtered={!!search}
                />
              )}
              <div className='min-h-0 flex-1 overflow-auto'>
                {rows.length > 0 ? (
                  <UpstreamUrlSummaryTable
                    rows={rows}
                    sort={sort}
                    onSort={(nextSort) => {
                      setSort(nextSort)
                      setPage(1)
                    }}
                    onSelect={(row) => setSelectedRow({ row, report })}
                    onEditDiscount={setDiscountTarget}
                  />
                ) : (
                  <p className='text-muted-foreground p-6'>
                    {t('No upstream usage')}
                  </p>
                )}
              </div>
              <BillingPagination
                label={t('Upstream pages')}
                page={page}
                pageSize={pageSize}
                total={query.data?.result.total ?? 0}
                disabled={query.isFetching}
                onChange={(next, size) => {
                  setPage(next)
                  setPageSize(size)
                }}
              />
            </>
          )}
        </>
      )}
      <Dialog
        open={selection != null}
        onOpenChange={(open) => {
          if (!open) setSelection(null)
        }}
        title={t('View details')}
        contentClassName='sm:max-w-6xl'
      >
        {selection && (
          <UpstreamDetailPanel
            key={JSON.stringify(selection)}
            period={props.period}
            selection={selection}
            onClose={() => setSelection(null)}
          />
        )}
      </Dialog>
      {discountTarget != null && (
        <UpstreamDiscountDialog
          channelId={discountTarget}
          month={props.month}
          periodStart={props.period.start_timestamp}
          onClose={() => setDiscountTarget(null)}
          onSaved={() => {
            const queryKey = [
              'billing-upstream-reconciliation',
              props.period.start_timestamp,
              props.period.end_timestamp,
            ]
            queryClient.removeQueries({ queryKey, type: 'inactive' })
            return queryClient.invalidateQueries({ queryKey })
          }}
        />
      )}
    </div>
  )
}
