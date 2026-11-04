import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import dayjs from 'dayjs'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { ProviderDiscount } from '@/features/billing-reconciliation/types'
import { upstreamDiscountSourceLabel } from '@/features/billing-reconciliation/upstream-reconciliation-utils'
import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

type Config = {
  channel_id: number
  channel_name: string
  upstream_name: string
  deleted: boolean
  expected_version: number
  discount: ProviderDiscount | null
}

// A single manual editor also accepts historical, deleted channel IDs.
export function ChannelBillingConfig() {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [idDraft, setIdDraft] = useState('')
  const [id, setId] = useState(0)
  const [month, setMonth] = useState(() =>
    dayjs()
      .add(8 * 60 + new Date().getTimezoneOffset(), 'minute')
      .format('YYYY-MM')
  )
  return (
    <Dialog
      open={open}
      onOpenChange={setOpen}
      title={t('Channel billing configuration')}
      description={t(
        'Enter a channel ID, including a deleted channel with historical usage. Names and discounts are maintained manually.'
      )}
      trigger={
        <Button variant='outline'>{t('Channel billing configuration')}</Button>
      }
    >
      <form
        className='flex items-end gap-2'
        onSubmit={(event) => {
          event.preventDefault()
          const value = Number(idDraft)
          if (!Number.isSafeInteger(value) || value <= 0) {
            toast.error(t('Enter a valid channel ID.'))
            return
          }
          setId(value)
        }}
      >
        <div className='space-y-1'>
          <Label htmlFor='billing-config-channel'>{t('Channel ID')}</Label>
          <Input
            id='billing-config-channel'
            inputMode='numeric'
            value={idDraft}
            onChange={(event) => setIdDraft(event.target.value)}
          />
        </div>
        <div className='space-y-1'>
          <Label htmlFor='billing-config-month'>{t('Billing month')}</Label>
          <Input
            id='billing-config-month'
            type='month'
            value={month}
            onChange={(event) => {
              if (event.target.value) setMonth(event.target.value)
            }}
          />
        </div>
        <Button type='submit'>{t('Load')}</Button>
      </form>
      {open && id > 0 && (
        <ChannelBillingEditor key={`${id}:${month}`} id={id} month={month} />
      )}
    </Dialog>
  )
}

function ChannelBillingEditor(props: { id: number; month: string }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [name, setName] = useState<string>()
  const [percent, setPercent] = useState<string>()
  const period = Date.parse(`${props.month}-01T00:00:00+08:00`) / 1000
  const query = useQuery({
    queryKey: ['channel-billing-config', props.id, period],
    queryFn: async () =>
      requireServerSuccess(
        (
          await api.get<{ success: boolean; message: string; data: Config }>(
            '/api/billing/admin/channel-config',
            { params: { channel_id: props.id, period_start: period } }
          )
        ).data
      ).data,
    retry: false,
  })
  const configuredPercent = query.data?.discount
    ? String(Number((Number(query.data.discount.value) * 100).toFixed(6)))
    : ''
  const save = useMutation({
    mutationFn: async (field: 'name' | 'discount') => {
      if (!query.data) throw new Error(t('Please try again later.'))
      if (field === 'name') {
        const value = (name ?? query.data.upstream_name).trim()
        if ([...value].length > 255) {
          throw new Error(t('Upstream name is too long.'))
        }
        requireServerSuccess(
          (
            await api.put('/api/billing/admin/channel-name', {
              channel_id: props.id,
              name: value,
            })
          ).data
        )
      } else {
        const value = (percent ?? configuredPercent).trim()
        if (
          !/^\d{1,3}(?:\.\d{1,6})?$/.test(value) ||
          Number(value) <= 0 ||
          Number(value) > 100
        ) {
          throw new Error(
            t('Enter a discount between 0% and 100% (exclusive of 0).')
          )
        }
        requireServerSuccess(
          (
            await api.put('/api/billing/admin/upstream-discounts', {
              channel_id: props.id,
              period_start: period,
              discount: (Number(value) / 100).toFixed(8),
              expected_version: query.data.expected_version,
            })
          ).data
        )
      }
      return field
    },
    onSuccess: async (field) => {
      if (field === 'name') setName(undefined)
      else setPercent(undefined)
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: ['channel-billing-config', props.id],
        }),
        queryClient.invalidateQueries({
          queryKey: ['billing-upstream-reconciliation'],
        }),
      ])
      toast.success(t('Saved successfully'))
    },
    onError: (error: Error) => toast.error(error.message),
  })
  if (query.isPending) return <LoadingState />
  if (query.isError || !query.data) {
    return (
      <ErrorState
        title={t('Unable to load upstream reconciliation')}
        onRetry={() => query.refetch()}
      />
    )
  }
  return (
    <div className='mt-4 space-y-4'>
      <p>
        {query.data.deleted
          ? t('Deleted channel')
          : query.data.channel_name || t('Channel')}{' '}
        #{props.id}
      </p>
      <div className='space-y-1'>
        <Label htmlFor='billing-upstream-name'>{t('Upstream name')}</Label>
        <div className='flex gap-2'>
          <Input
            id='billing-upstream-name'
            value={name ?? query.data.upstream_name}
            onChange={(event) => setName(event.target.value)}
          />
          <Button disabled={save.isPending} onClick={() => save.mutate('name')}>
            {t('Save name')}
          </Button>
        </div>
      </div>
      <div className='space-y-1'>
        <Label htmlFor='billing-discount-percent'>
          {t('Discount percent')}
        </Label>
        <div className='flex gap-2'>
          <Input
            id='billing-discount-percent'
            inputMode='decimal'
            value={percent ?? configuredPercent}
            onChange={(event) => setPercent(event.target.value)}
          />
          <Button
            disabled={save.isPending}
            onClick={() => save.mutate('discount')}
          >
            {t('Save discount')}
          </Button>
        </div>
        <p className='text-muted-foreground text-sm'>
          {upstreamDiscountSourceLabel(query.data.discount, t)}
        </p>
      </div>
    </div>
  )
}
