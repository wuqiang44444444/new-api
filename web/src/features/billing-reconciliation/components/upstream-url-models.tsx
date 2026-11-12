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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import { billingModeLabel } from '../lib'
import type { UpstreamUrlModelRow } from '../types'
import { useUpstreamPage } from '../upstream-page-query'
import { upstreamModelLabel } from '../upstream-statement-utils'
import { BillingPagination } from './billing-pagination'
import { UpstreamDataStatus } from './upstream-data-status'
import { UpstreamDiscountValue } from './upstream-discount-value'
import {
  AmountCell,
  UsageCell,
  type UpstreamDetailViewSelection,
} from './upstream-reconciliation-table'
import { UpstreamUsageValues } from './upstream-usage-values'

type Status = '' | 'complete' | 'partial'

export function UpstreamUrlModels(props: {
  urlKey: string
  channelId: number
  reportVersion?: string
  period: { start_timestamp: number; end_timestamp: number }
  onViewDetails: (selection: UpstreamDetailViewSelection) => void
}) {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [modelDraft, setModelDraft] = useState('')
  const [search, setSearch] = useState('')
  const [status, setStatus] = useState<Status>('')
  const query = useUpstreamPage({
    ...props.period,
    level: 'url_models',
    report_version: props.reportVersion,
    url_key: props.reportVersion ? props.urlKey : undefined,
    channel_id: props.channelId,
    data_status: status || undefined,
    search,
    page,
    page_size: pageSize,
  })
  const rows = query.data?.result.model_rows ?? []
  const total = query.data?.result.total ?? 0
  return (
    <>
      <div className='flex shrink-0 flex-wrap items-center gap-2'>
        <NativeSelect
          aria-label={t('Data status')}
          value={status}
          onChange={(event) => {
            setStatus(event.target.value as Status)
            setPage(1)
          }}
        >
          <NativeSelectOption value=''>{t('All statuses')}</NativeSelectOption>
          <NativeSelectOption value='complete'>
            {t('Complete')}
          </NativeSelectOption>
          <NativeSelectOption value='partial'>
            {t('Partial')}
          </NativeSelectOption>
        </NativeSelect>
        <form
          className='flex min-w-0 flex-wrap items-center gap-2'
          onSubmit={(event) => {
            event.preventDefault()
            setSearch(modelDraft.trim())
            setPage(1)
          }}
        >
          <Input
            aria-label={t('Provider model')}
            placeholder={t('Provider model')}
            value={modelDraft}
            maxLength={255}
            onChange={(event) => setModelDraft(event.target.value)}
            className='w-44'
          />
          <Button variant='outline' size='sm' type='submit'>
            {t('Search')}
          </Button>
        </form>
      </div>
      {query.isPending && <LoadingState />}
      {query.isError && (
        <ErrorState
          title={t('Unable to load upstream details.')}
          onRetry={() => query.refetch()}
        />
      )}
      {query.isSuccess && rows.length === 0 && (
        <p className='text-muted-foreground p-6'>{t('No matching results')}</p>
      )}
      {query.isSuccess && rows.length > 0 && (
        <div className='min-h-0 flex-1 overflow-auto'>
          <Table withContainer={false}>
            <TableHeader className='sticky top-0 z-10 bg-(--table-header)'>
              <TableRow>
                <TableHead scope='col'>{t('Channel')}</TableHead>
                <TableHead scope='col'>{t('Provider model')}</TableHead>
                <TableHead scope='col'>{t('Billing mode')}</TableHead>
                <TableHead scope='col' className='text-right'>
                  {t('Requests')}
                </TableHead>
                <TableHead scope='col'>{t('Usage')}</TableHead>
                <TableHead
                  scope='col'
                  className='text-right'
                  title={t('Original amount (local official price)')}
                >
                  {t('List price')}
                </TableHead>
                <TableHead scope='col'>{t('Channel discount')}</TableHead>
                <TableHead scope='col' className='text-right'>
                  {t('Discounted reference amount')}
                </TableHead>
                <TableHead scope='col'>{t('Data status')}</TableHead>
                <TableHead
                  scope='col'
                  className='sticky right-0 bg-(--table-header) text-right'
                >
                  {t('Actions')}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((row) => (
                <UpstreamUrlModelTableRow
                  key={JSON.stringify([
                    row.channel_id,
                    row.provider_model,
                    row.provider_model_fallback ?? false,
                    row.billing_mode,
                  ])}
                  row={row}
                  onViewDetails={props.onViewDetails}
                />
              ))}
            </TableBody>
          </Table>
        </div>
      )}
      {query.isSuccess && total > 0 && (
        <BillingPagination
          label={t('Model pages')}
          page={page}
          pageSize={pageSize}
          total={total}
          disabled={query.isFetching}
          onChange={(next, size) => {
            setPage(next)
            setPageSize(size)
          }}
        />
      )}
    </>
  )
}

function UpstreamUrlModelTableRow(props: {
  row: UpstreamUrlModelRow
  onViewDetails: (selection: UpstreamDetailViewSelection) => void
}) {
  const { t } = useTranslation()
  const row = props.row
  const selection = {
    channelId: row.channel_id,
    providerModel: row.provider_model,
    providerModelFallback: row.provider_model_fallback ?? false,
    billingMode: row.billing_mode,
  }
  return (
    <TableRow>
      <TableCell className='max-w-52 min-w-32 wrap-anywhere whitespace-normal'>
        {row.upstream_name || row.channel_name || t('Deleted channel')}
        <span
          className='text-muted-foreground block'
          data-table-text='secondary'
        >
          {row.upstream_name &&
            row.channel_name &&
            row.upstream_name !== row.channel_name &&
            `${row.channel_name} · `}
          #{row.channel_id}
          {row.deleted && ` · ${t('Deleted channel')}`}
        </span>
      </TableCell>
      <TableCell className='max-w-56 min-w-32 wrap-anywhere whitespace-normal'>
        {upstreamModelLabel(row)}
      </TableCell>
      <TableCell>{t(billingModeLabel(row.billing_mode))}</TableCell>
      <UsageCell value={row.usage.requests} />
      <TableCell>
        <UpstreamUsageValues
          usage={row.usage}
          quality={row.data_quality}
          billingMode={row.billing_mode}
          showCache
        />
      </TableCell>
      <AmountCell
        value={row.original_amount}
        knownSubtotal={row.known_original_amount}
        estimated={row.data_quality?.has_estimated_original}
      />
      <TableCell>
        <UpstreamDiscountValue discount={row.discount} />
      </TableCell>
      <AmountCell
        value={row.reference_amount}
        knownSubtotal={row.known_reference_amount}
      />
      <TableCell>
        <UpstreamDataStatus
          quality={row.data_quality}
          originalAmount={row.original_amount}
          usageOnly={row.usage_only}
          onViewEvidence={(entry) =>
            props.onViewDetails({
              ...selection,
              evidenceFilter: entry.filter,
              evidenceLabel: entry.text,
            })
          }
        />
      </TableCell>
      <TableCell className='bg-background sticky right-0 border-l text-right'>
        <Button
          variant='link'
          size='xs'
          onClick={() => props.onViewDetails(selection)}
        >
          {t('View records')}
        </Button>
      </TableCell>
    </TableRow>
  )
}
