import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { channelBillingConfigQuery } from '@/features/billing-reconciliation/channel-billing-config-api'
import {
  currentShanghaiMonth,
  resolveShanghaiMonth,
} from '@/features/billing-reconciliation/lib'
import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

export function ChannelUpstreamNameConfig() {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [idDraft, setIdDraft] = useState('')
  const [id, setId] = useState(0)
  return (
    <Dialog
      open={open}
      onOpenChange={setOpen}
      title={t('Edit upstream name')}
      trigger={<Button variant='outline'>{t('Edit upstream name')}</Button>}
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
        <Button type='submit'>{t('Load')}</Button>
      </form>
      {open && id > 0 && <ChannelUpstreamNameEditor key={id} id={id} />}
    </Dialog>
  )
}

function ChannelUpstreamNameEditor(props: { id: number }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [name, setName] = useState<string>()
  const period = resolveShanghaiMonth(currentShanghaiMonth())
  const query = useQuery(
    channelBillingConfigQuery(props.id, period.start_timestamp)
  )
  const save = useMutation({
    mutationFn: async () => {
      if (!query.data) throw new Error(t('Please try again later.'))
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
    },
    onSuccess: async () => {
      setName(undefined)
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
  if (!query.data) {
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
        {query.data.channel_name || t('Channel')} · #{props.id}
        {query.data.deleted && ` · ${t('Deleted channel')}`}
      </p>
      <div className='space-y-1'>
        <Label htmlFor='billing-upstream-name'>{t('Upstream name')}</Label>
        <div className='flex gap-2'>
          <Input
            id='billing-upstream-name'
            value={name ?? query.data.upstream_name}
            onChange={(event) => setName(event.target.value)}
          />
          <Button disabled={save.isPending} onClick={() => save.mutate()}>
            {t('Save name')}
          </Button>
        </div>
      </div>
    </div>
  )
}
