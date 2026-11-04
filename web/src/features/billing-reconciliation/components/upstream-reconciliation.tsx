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
import { useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

import { createUpstreamExport } from '../export-api'
import { ExportJobsDrawer } from '../export-jobs-drawer'
import { useUpstreamPage } from '../upstream-page-query'
import {
  billingEvidenceGroups,
  billingAccountingEntries,
} from '../upstream-statement-utils'
import { BillingPagination } from './billing-pagination'
import {
  UpstreamDetailPanel,
  type UpstreamDetailSelection,
} from './upstream-detail-panel'
import { UpstreamEvidenceCoverage } from './upstream-evidence-coverage'
import { UpstreamReconciliationTable } from './upstream-reconciliation-table'

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
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [selection, setSelection] = useState<UpstreamDetailSelection | null>(
    null
  )
  const [exportsOpen, setExportsOpen] = useState(false)
  const [exporting, setExporting] = useState(false)
  const query = useUpstreamPage({
    ...props.period,
    level: 'channels',
    search,
    page,
    page_size: pageSize,
  })
  const exportChannel = async (id: number) => {
    if (exporting) return
    setExporting(true)
    try {
      await createUpstreamExport({
        job_type: 'upstream_summary',
        channel_id: id,
        start_timestamp: props.period.start_timestamp,
        end_timestamp: props.period.end_timestamp + 1,
      })
      await queryClient.invalidateQueries({
        queryKey: ['customer-export-jobs'],
      })
      setExportsOpen(true)
      toast.success(t('Export submitted. Track it in export jobs.'))
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Unable to submit export.')
      )
    } finally {
      setExporting(false)
    }
  }
  const quality = query.data?.result.data_quality
  return (
    <div className='flex min-h-0 flex-1 flex-col gap-4'>
      <div className='flex flex-wrap items-end gap-3'>
        <div className='space-y-1'>
          <Label htmlFor='upstream-billing-month'>{t('Billing month')}</Label>
          <Input
            id='upstream-billing-month'
            type='month'
            value={props.month}
            onChange={(event) => {
              if (event.target.value) props.onMonthChange(event.target.value)
            }}
          />
        </div>
        <form
          className='flex items-end gap-2'
          onSubmit={(event) => {
            event.preventDefault()
            setSearch(searchDraft.trim())
            setPage(1)
            setExpanded(new Set())
          }}
        >
          <div className='space-y-1'>
            <Label htmlFor='upstream-channel-search'>
              {t('Channel name or ID')}
            </Label>
            <Input
              id='upstream-channel-search'
              value={searchDraft}
              maxLength={255}
              onChange={(event) => setSearchDraft(event.target.value)}
            />
          </div>
          <Button type='submit' variant='outline'>
            {t('Search')}
          </Button>
        </form>
        <Button variant='outline' onClick={() => setExportsOpen(true)}>
          {t('Export jobs')}
        </Button>
      </div>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Upstream names and monthly discounts are entered manually in channel management. Calculated amounts are references, not supplier bills. Missing amounts remain unknown.'
        )}
      </p>
      {query.isPending && <LoadingState />}
      {query.isError && (
        <ErrorState
          title={t('Unable to load upstream reconciliation')}
          onRetry={() => query.refetch()}
        />
      )}
      {!query.isPending && !query.isError && (
        <>
          <UpstreamEvidenceCoverage
            quality={quality}
            onViewEvidence={(entry) =>
              setSelection({
                evidenceFilter: entry.filter,
                evidenceLabel: entry.text,
              })
            }
          />
          {billingEvidenceGroups(quality, t).map((group) => (
            <div key={group.key} className='flex flex-wrap gap-2'>
              {group.title && <span>{group.title}</span>}
              {group.entries.map((entry) => (
                <Button
                  key={entry.filter || entry.text}
                  size='sm'
                  variant='link'
                  onClick={() =>
                    setSelection({
                      evidenceFilter: entry.filter,
                      evidenceLabel: entry.text,
                    })
                  }
                >
                  {entry.text}
                </Button>
              ))}
            </div>
          ))}
          {billingAccountingEntries(quality, t).length > 0 && (
            <div>
              <p>{t('Confirmed accounting results')}</p>
              {billingAccountingEntries(quality, t).map((entry) => (
                <Button
                  key={entry.filter || entry.text}
                  size='sm'
                  variant='link'
                  onClick={() =>
                    setSelection({
                      evidenceFilter: entry.filter,
                      evidenceLabel: entry.text,
                    })
                  }
                >
                  {entry.text}
                </Button>
              ))}
            </div>
          )}
          <div className='min-h-0 overflow-auto'>
            {query.data?.result.channels.length ? (
              <UpstreamReconciliationTable
                channels={query.data.result.channels}
                periodStart={props.period.start_timestamp}
                periodEnd={props.period.end_timestamp}
                expandedChannels={expanded}
                onToggleChannel={(id) =>
                  setExpanded((current) => {
                    const next = new Set(current)
                    if (next.has(id)) next.delete(id)
                    else next.add(id)
                    return next
                  })
                }
                onViewDetails={setSelection}
                onExportChannel={(id) => {
                  void exportChannel(id)
                }}
                exporting={exporting}
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
              setExpanded(new Set())
            }}
          />
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
            onShowExports={() => setExportsOpen(true)}
          />
        )}
      </Dialog>
      <ExportJobsDrawer open={exportsOpen} onOpenChange={setExportsOpen} />
    </div>
  )
}
