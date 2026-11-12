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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { isAxiosError } from 'axios'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { getChannels, searchChannels } from '@/features/channels/api'
import { api } from '@/lib/api'
import {
  getServerErrorMessage,
  requireServerSuccess,
} from '@/lib/server-error-message'

import { channelBillingConfigQuery } from '../channel-billing-config-api'
import type { ApiResponse } from '../types'
import {
  formatShanghaiTimestamp,
  upstreamDiscountLabel,
  upstreamDiscountSourceLabel,
} from '../upstream-reconciliation-utils'
import { BillingPagination } from './billing-pagination'
import { UpstreamDiscountRanges } from './upstream-discount-value'

type Props = {
  channelId: number
  month: string
  periodStart: number
  onClose: () => void
  onSaved: () => Promise<void>
}

export function UpstreamDiscountDialog(props: Props) {
  const { t } = useTranslation()
  const [id, setId] = useState(props.channelId)
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose()
      }}
      title={t('Set discount')}
      description={t(
        'This discount applies to all URLs for this channel in {{month}}.',
        {
          month: props.month,
        }
      )}
    >
      {id > 0 ? (
        <UpstreamDiscountEditor
          key={id}
          {...props}
          channelId={id}
          onChangeChannel={props.channelId ? undefined : () => setId(0)}
        />
      ) : (
        <DiscountChannelPicker onSelect={setId} />
      )}
    </Dialog>
  )
}

function DiscountChannelPicker({
  onSelect,
}: {
  onSelect: (id: number) => void
}) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState('')
  const [keyword, setKeyword] = useState('')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const query = useQuery({
    queryKey: ['billing-discount-channels', keyword, page, pageSize],
    queryFn: async () => {
      const params = { p: page, page_size: pageSize }
      const response = requireServerSuccess(
        keyword
          ? await searchChannels({ ...params, keyword })
          : await getChannels(params)
      )
      if (!response.data) throw new Error(t('Please try again later.'))
      return {
        total: response.data.total,
        items: response.data.items.map(({ id, name, base_url }) => {
          let host = ''
          if (base_url) {
            try {
              host = new URL(base_url).host
            } catch {
              // The channel name and ID still identify nonstandard URLs.
            }
          }
          return { id, name, host }
        }),
      }
    },
    retry: false,
  })
  const exactId = /^\d+$/.test(keyword) ? Number(keyword) : 0
  return (
    <div className='space-y-3'>
      <form
        className='flex items-end gap-2'
        onSubmit={(event) => {
          event.preventDefault()
          setPage(1)
          setKeyword(draft.trim())
        }}
      >
        <div className='min-w-0 flex-1 space-y-1'>
          <Label htmlFor='upstream-discount-channel'>{t('Channel')}</Label>
          <Input
            id='upstream-discount-channel'
            value={draft}
            maxLength={255}
            placeholder={t('Channel name, URL, or ID')}
            onChange={(event) => setDraft(event.target.value)}
          />
        </div>
        <Button type='submit' variant='outline'>
          {t('Search')}
        </Button>
      </form>
      {query.isPending && <LoadingState />}
      {query.isError && (
        <ErrorState
          title={t('Unable to load upstream reconciliation')}
          onRetry={() => query.refetch()}
        />
      )}
      {query.isSuccess && (
        <>
          {query.data.items.length === 0 ? (
            <div className='space-y-2'>
              <p className='text-muted-foreground text-sm' role='status'>
                {t('No matching channels')}
              </p>
              {Number.isSafeInteger(exactId) && exactId > 0 && (
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={() => onSelect(exactId)}
                >
                  {t('Look up channel #{{id}}', { id: exactId })}
                </Button>
              )}
            </div>
          ) : (
            <ul
              className='max-h-60 divide-y overflow-auto rounded-md border'
              aria-label={t('Channels')}
            >
              {query.data.items.map(({ id, name, host }) => (
                <li key={id}>
                  <Button
                    type='button'
                    variant='ghost'
                    className='h-auto min-h-10 w-full justify-between gap-3 rounded-none px-3 py-2 text-start whitespace-normal'
                    onClick={() => onSelect(id)}
                  >
                    <span className='min-w-0 wrap-anywhere'>
                      <span className='block'>{name || t('Channel')}</span>
                      {host && (
                        <span className='text-muted-foreground block text-xs'>
                          {host}
                        </span>
                      )}
                    </span>
                    <span className='text-muted-foreground shrink-0 text-xs'>
                      #{id}
                    </span>
                  </Button>
                </li>
              ))}
            </ul>
          )}
          {query.data.total > 0 && (
            <BillingPagination
              page={page}
              pageSize={pageSize}
              total={query.data.total}
              label={t('Channels')}
              disabled={query.isFetching}
              onChange={(nextPage, nextSize) => {
                setPage(nextPage)
                setPageSize(nextSize)
              }}
            />
          )}
        </>
      )}
    </div>
  )
}

function UpstreamDiscountEditor(
  props: Props & { onChangeChannel?: () => void }
) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [draft, setDraft] = useState<{ percent: string; basis: string }>()
  const [scope, setScope] = useState<'month' | 'date'>('month')
  const [effectiveDate, setEffectiveDate] = useState(() => {
    const today = formatShanghaiTimestamp(Date.now() / 1000).slice(0, 10)
    return today.startsWith(`${props.month}-`) ? today : `${props.month}-01`
  })
  const monthFirstDay = `${props.month}-01`
  const monthAfter = new Date(props.periodStart * 1000 + 8 * 3600_000)
  monthAfter.setUTCMonth(monthAfter.getUTCMonth() + 1)
  const monthLastDay = new Date(monthAfter.getTime() - 86400_000)
    .toISOString()
    .slice(0, 10)
  const startDate = scope === 'month' ? monthFirstDay : effectiveDate
  const validDate =
    /^\d{4}-\d{2}-\d{2}$/.test(startDate) &&
    startDate >= monthFirstDay &&
    startDate <= monthLastDay
  const startsAt = Date.parse(`${startDate}T00:00:00+08:00`) / 1000
  const replacesMonth = startsAt === props.periodStart
  const [conflicted, setConflicted] = useState(false)
  const query = useQuery({
    ...channelBillingConfigQuery(props.channelId, props.periodStart),
    staleTime: 0,
  })
  const configuredPercent = query.data?.discount
    ? String(Number((Number(query.data.discount.value) * 100).toFixed(6)))
    : ''
  const basis = JSON.stringify([
    query.data?.expected_version,
    query.data?.discount,
  ])
  const needsRefresh = conflicted || (draft != null && draft.basis !== basis)
  const percent = draft?.percent ?? configuredPercent
  const validPercent =
    /^\d{1,3}(?:\.\d{1,6})?$/.test(percent.trim()) &&
    Number(percent) > 0 &&
    Number(percent) <= 100
  const save = useMutation({
    mutationFn: async () => {
      if (!query.data) throw new Error(t('Please try again later.'))
      const value = percent.trim()
      if (
        !/^\d{1,3}(?:\.\d{1,6})?$/.test(value) ||
        Number(value) <= 0 ||
        Number(value) > 100
      ) {
        throw new Error(
          t('Enter a discount between 0% and 100% (exclusive of 0).')
        )
      }
      const response = await api.put<ApiResponse<{ version: number }>>(
        '/api/billing/admin/upstream-discounts',
        {
          channel_id: props.channelId,
          period_start: props.periodStart,
          discount: (Number(value) / 100).toFixed(8),
          expected_version: query.data.expected_version,
          ...(scope === 'date' ? { effective_date: effectiveDate } : {}),
        },
        { skipErrorHandler: true }
      )
      const saved = requireServerSuccess(response.data).data
      if (!saved) throw new Error(t('Please try again later.'))
    },
    onSuccess: async () => {
      void queryClient.invalidateQueries({
        queryKey: ['channel-billing-config', props.channelId],
      })
      await props.onSaved()
      toast.success(t('Saved successfully'))
      props.onClose()
    },
    onError: (error) => {
      if (isAxiosError(error) && error.response?.status === 409) {
        setConflicted(true)
        return
      }
      toast.error(getServerErrorMessage(error, t('Please try again later.')))
    },
  })
  const changeChannel = props.onChangeChannel && (
    <Button
      type='button'
      variant='outline'
      size='sm'
      disabled={save.isPending}
      onClick={props.onChangeChannel}
    >
      {t('Change channel')}
    </Button>
  )
  if (!query.data) {
    return (
      <div className='space-y-3'>
        {changeChannel}
        {query.isPending ? (
          <LoadingState />
        ) : (
          <ErrorState
            title={t('Unable to load upstream reconciliation')}
            onRetry={() => query.refetch()}
          />
        )}
      </div>
    )
  }
  const current = query.data.discount
  const changes = current?.changes ?? []
  const previewValue = (Number(percent) / 100).toFixed(8)
  const retainedChanges = changes.filter(
    (change) => change.effective_from < startsAt
  )
  const previewBase = replacesMonth ? previewValue : (current?.value ?? '')
  const previousValue = retainedChanges.at(-1)?.value ?? current?.value
  const previewChanges = replacesMonth ? [] : retainedChanges
  if (!replacesMonth && Number(previousValue) !== Number(previewValue)) {
    previewChanges.push({ effective_from: startsAt, value: previewValue })
  }
  const replacesChanges = changes.some(
    (change) => change.effective_from >= startsAt
  )
  const canPreview =
    validPercent && validDate && (replacesMonth || current != null)
  const scheduleUnchanged =
    query.data.expected_version > 0 &&
    Number(previewBase) === Number(current?.value) &&
    previewChanges.length === changes.length &&
    previewChanges.every(
      (change, index) =>
        change.effective_from === changes[index].effective_from &&
        Number(change.value) === Number(changes[index].value)
    )
  const saveDisabled =
    needsRefresh ||
    query.isFetching ||
    query.isError ||
    save.isPending ||
    !canPreview ||
    scheduleUnchanged
  return (
    <form
      className='space-y-3'
      onSubmit={(event) => {
        event.preventDefault()
        if (!saveDisabled) save.mutate()
      }}
    >
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <div className='min-w-0 text-sm wrap-anywhere'>
          <p className='font-medium'>
            {query.data.channel_name ||
              query.data.upstream_name ||
              t('Channel')}{' '}
            · #{props.channelId}
            {query.data.deleted && ` · ${t('Deleted channel')}`}
          </p>
          {query.data.upstream_name &&
            query.data.channel_name &&
            query.data.upstream_name !== query.data.channel_name && (
              <p className='text-muted-foreground text-xs'>
                {query.data.upstream_name}
              </p>
            )}
        </div>
        {changeChannel}
      </div>
      <div className='space-y-1 rounded-md border p-3' aria-live='polite'>
        <p className='text-muted-foreground text-xs'>{t('Current discount')}</p>
        {current && changes.length > 0 ? (
          <UpstreamDiscountRanges
            start={props.periodStart}
            value={current.value}
            changes={changes}
          />
        ) : (
          <p className='text-sm font-medium'>
            {upstreamDiscountLabel(current, t)}
          </p>
        )}
        <p className='text-muted-foreground text-xs'>
          {upstreamDiscountSourceLabel(current, t)}
        </p>
      </div>
      <div className='flex flex-wrap items-end gap-3'>
        <div className='space-y-1'>
          <Label htmlFor='upstream-discount-scope'>
            {t('Discount effective range')}
          </Label>
          <NativeSelect
            id='upstream-discount-scope'
            value={scope}
            disabled={save.isPending}
            onChange={(event) =>
              setScope(event.target.value === 'date' ? 'date' : 'month')
            }
          >
            <NativeSelectOption value='month'>
              {t('Entire billing month')}
            </NativeSelectOption>
            <NativeSelectOption value='date'>
              {t('From a specific date')}
            </NativeSelectOption>
          </NativeSelect>
        </div>
        {scope === 'date' && (
          <div className='space-y-1'>
            <Label htmlFor='upstream-discount-date'>
              {t('Effective date (Beijing time)')}
            </Label>
            <Input
              id='upstream-discount-date'
              type='date'
              min={monthFirstDay}
              max={monthLastDay}
              value={effectiveDate}
              disabled={save.isPending}
              onChange={(event) => setEffectiveDate(event.target.value)}
            />
          </div>
        )}
      </div>
      <div className='space-y-1'>
        <Label htmlFor='upstream-discount-percent'>
          {t('Discount percent')}
        </Label>
        <Input
          id='upstream-discount-percent'
          inputMode='decimal'
          value={percent}
          disabled={save.isPending}
          aria-describedby='upstream-discount-percent-help'
          aria-invalid={draft != null && !validPercent}
          onChange={(event) =>
            setDraft({
              percent: event.target.value,
              basis: draft?.basis ?? basis,
            })
          }
        />
        <p
          id='upstream-discount-percent-help'
          className='text-muted-foreground text-xs'
        >
          {draft != null && !validPercent
            ? t('Enter a discount between 0% and 100% (exclusive of 0).')
            : t(
                'Enter 80 to apply 80% of the original amount. Enter 100 for no discount.'
              )}
        </p>
      </div>
      {scope === 'date' && !validDate && (
        <p className='text-destructive text-sm'>
          {t('Choose a date within the selected billing month.')}
        </p>
      )}
      {validDate && !replacesMonth && !current && (
        <p className='text-muted-foreground text-sm'>
          {t('Set an entire month discount before adding a dated change.')}
        </p>
      )}
      {canPreview && (
        <div className='space-y-1 rounded-md border p-3'>
          <p className='text-sm font-medium'>{t('Discount after saving')}</p>
          <UpstreamDiscountRanges
            start={props.periodStart}
            value={String(Number(previewBase))}
            changes={previewChanges.map((change) => ({
              ...change,
              value: String(Number(change.value)),
            }))}
          />
          {replacesChanges && (
            <p className='text-muted-foreground text-xs'>
              {replacesMonth
                ? t('This replaces all discount changes in the selected month.')
                : t(
                    'This replaces scheduled discount changes from the selected date through month end.'
                  )}
            </p>
          )}
        </div>
      )}
      {(needsRefresh || query.isError) && (
        <div className='space-y-2 text-sm' role='status'>
          <p>
            {needsRefresh
              ? t(
                  'The discount changed. Refresh the saved value before trying again.'
                )
              : t('Refresh failed, showing last synced value')}
          </p>
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={query.isFetching}
            onClick={async () => {
              const result = await query.refetch()
              if (result.isSuccess && result.data) {
                const data = result.data
                setDraft(
                  (current) =>
                    current && {
                      ...current,
                      basis: JSON.stringify([
                        data.expected_version,
                        data.discount,
                      ]),
                    }
                )
                setConflicted(false)
              } else {
                toast.error(t('Refresh failed'))
              }
            }}
          >
            {t('Refresh')}
          </Button>
        </div>
      )}
      <Button type='submit' disabled={saveDisabled}>
        {t('Save discount')}
      </Button>
    </form>
  )
}
