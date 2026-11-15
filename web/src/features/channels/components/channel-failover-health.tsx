import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState, type FormEvent, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { toIntlLocale } from '@/i18n/languages'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { api } from '@/lib/api'
import { handleServerError } from '@/lib/handle-server-error'
import { useAuthStore } from '@/stores/auth-store'

import { isChannelEnabled, isMultiKeyChannel } from '../lib'
import type { Channel } from '../types'

type Operation = 'messages' | 'chat' | 'responses'
type Action = 'probe' | 'permit_trial' | 'bypass' | 'clear_bypass'

interface HealthRow {
  scope: {
    channel_id: number
    model: string
    operation: Operation
    slot: number
  }
  enabled: boolean
  enforced: boolean
  state: '' | 'cooldown' | 'pending' | 'trial_ready'
  next_check: string
  updated_at: string
  manual_until: string
  manual_reason?: string
  probe_result?: string
  probe_status?: number
  trial_result?: string
  trial_status?: number
  samples?: { status: number; at: string }[]
}

const operationLabels = {
  messages: 'Claude Messages',
  chat: 'Chat Completions',
  responses: 'Responses',
} as const

export function ChannelFailoverDialog({
  channel,
  onOpenChange,
}: {
  channel: Channel
  onOpenChange: (open: boolean) => void
}) {
  const currentUser = useAuthStore((s) => s.auth.user)
  const canOperate =
    isChannelEnabled(channel) &&
    !isMultiKeyChannel(channel) &&
    hasPermission(
      currentUser,
      ADMIN_PERMISSION_RESOURCES.CHANNEL,
      ADMIN_PERMISSION_ACTIONS.OPERATE
    )
  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className='max-h-[85dvh] overflow-y-auto sm:max-w-2xl'>
        <DialogHeader>
          <DialogTitle>{channel.name}</DialogTitle>
        </DialogHeader>
        <ChannelFailoverHealth
          channelId={channel.id}
          models={channel.models
            .split(',')
            .map((model) => model.trim())
            .filter(Boolean)}
          canOperate={canOperate}
        />
      </DialogContent>
    </Dialog>
  )
}

export function ChannelFailoverHealth({
  channelId,
  models = [],
  canOperate = false,
}: {
  channelId?: number
  models?: string[]
  canOperate?: boolean
}) {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const [operation, setOperation] = useState<Operation>('messages')
  const [chosenModel, setChosenModel] = useState('')
  const [minutes, setMinutes] = useState('60')
  const [reason, setReason] = useState('')
  const model = chosenModel || models[0] || ''
  const path = channelId
    ? `/api/channel/${channelId}/failover`
    : '/api/channel/failover'
  const query = useQuery({
    queryKey: ['channel-failover-health', channelId ?? null],
    queryFn: async () => {
      const { data } = await api.get<{ success: boolean; data?: HealthRow[] }>(
        path
      )
      if (!data.success || !Array.isArray(data.data)) {
        throw new Error('channel failover health unavailable')
      }
      return data.data
    },
  })
  const mutation = useMutation({
    mutationFn: async (action: Action) => {
      const { data } = await api.post<{ success: boolean; message?: string }>(
        path,
        {
          action,
          operation,
          model,
          ...(action === 'bypass'
            ? { minutes: Number(minutes), reason: reason.trim() }
            : {}),
        }
      )
      if (!data.success) throw new Error(data.message || t('Unavailable'))
    },
    onSuccess: async () => {
      toast.success(t('Saved'))
      await queryClient.invalidateQueries({
        queryKey: ['channel-failover-health'],
      })
    },
    onError: (error) => handleServerError(error),
  })

  const formatTime = (raw: string) => {
    const date = new Date(raw)
    return Number.isNaN(date.valueOf()) || date.getUTCFullYear() < 2000
      ? '—'
      : date.toLocaleString(
          toIntlLocale(i18n.resolvedLanguage || i18n.language)
        )
  }

  const statusLabel = (row: HealthRow) => {
    if (!row.enabled) return t('Channel disabled')
    if (!row.enforced) return t('Observe only')
    if (Date.parse(row.manual_until) > Date.now()) {
      return t('Temporarily bypassed')
    }
    switch (row.state) {
      case 'cooldown':
        return t('In cooldown')
      case 'pending':
        return t('Recovery pending confirmation')
      case 'trial_ready':
        return t('Ready for controlled trial')
      default:
        return t('Available')
    }
  }

  const probeLabel = (result?: string) => {
    switch (result) {
      case 'success':
        return t('Probe passed')
      case 'failed':
        return t('Probe failed')
      case 'unconfirmed':
        return t('Recovery pending confirmation')
      case 'manual_trial':
        return t('Trial permitted by an operator')
      default:
        return '—'
    }
  }

  const trialLabel = (result?: string) => {
    switch (result) {
      case 'success':
        return t('Recovery trial succeeded')
      case 'failed':
        return t('Recovery trial failed')
      case 'unconfirmed':
        return t('Recovery trial unconfirmed')
      default:
        return '—'
    }
  }

  let healthContent: ReactNode
  if (query.isPending) {
    healthContent = <p>{t('Loading...')}</p>
  } else if (query.isError) {
    healthContent = <p>{t('Unavailable')}</p>
  } else if (query.data.length === 0) {
    healthContent = (
      <p className='text-muted-foreground text-sm'>
        {t('No failover evidence recorded')}
      </p>
    )
  } else {
    healthContent = query.data.map((row) => (
      <div
        key={`${row.scope.channel_id}-${row.scope.model}-${row.scope.operation}-${row.scope.slot}`}
        className='grid gap-2 rounded-md border p-3 text-sm'
      >
        <p className='font-medium break-all'>
          #{row.scope.channel_id} · {row.scope.model} ·{' '}
          {operationLabels[row.scope.operation]}
          {row.scope.slot >= 0
            ? ` · ${t('Key slot {{number}}', { number: row.scope.slot + 1 })}`
            : ''}
        </p>
        <dl className='grid gap-2 sm:grid-cols-2'>
          <div>
            <dt className='text-muted-foreground'>{t('Routing status')}</dt>
            <dd>{statusLabel(row)}</dd>
          </div>
          <div>
            <dt className='text-muted-foreground'>
              {t('Next recovery check')}
            </dt>
            <dd>{formatTime(row.next_check)}</dd>
          </div>
          <div>
            <dt className='text-muted-foreground'>{t('Probe result')}</dt>
            <dd>
              {probeLabel(row.probe_result)}
              {row.probe_status ? ` (${row.probe_status})` : ''}
            </dd>
          </div>
          <div>
            <dt className='text-muted-foreground'>{t('Recovery trial')}</dt>
            <dd>
              {trialLabel(row.trial_result)}
              {row.trial_status ? ` (${row.trial_status})` : ''}
            </dd>
          </div>
          <div>
            <dt className='text-muted-foreground'>{t('Failure evidence')}</dt>
            <dd>
              {row.samples
                ?.map((sample) => sample.status || t('Connection failure'))
                .join(', ') || '—'}
            </dd>
          </div>
        </dl>
        {row.manual_reason && Date.parse(row.manual_until) > Date.now() && (
          <p className='break-words'>
            {row.manual_reason} · {formatTime(row.manual_until)}
          </p>
        )}
      </div>
    ))
  }

  const submitBypass = (event: FormEvent) => {
    event.preventDefault()
    if (reason.trim() && model) mutation.mutate('bypass')
  }

  return (
    <section aria-label={t('Channel failover')} className='min-w-0 space-y-4'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <h3 className='text-sm font-semibold'>{t('Channel failover')}</h3>
        <Button
          type='button'
          variant='outline'
          size='sm'
          disabled={query.isFetching}
          onClick={() => void query.refetch()}
        >
          {t('Refresh')}
        </Button>
      </div>
      <div aria-live='polite' className='space-y-2'>
        {healthContent}
      </div>
      {channelId && canOperate && models.length > 0 ? (
        <form className='space-y-3 border-t pt-3' onSubmit={submitBypass}>
          <div className='grid gap-3 sm:grid-cols-2'>
            <div className='grid gap-1.5'>
              <Label htmlFor='failover-model'>{t('Model')}</Label>
              <NativeSelect
                id='failover-model'
                className='w-full'
                value={model}
                onChange={(event) => setChosenModel(event.target.value)}
              >
                {models.map((item) => (
                  <NativeSelectOption key={item} value={item}>
                    {item}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </div>
            <div className='grid gap-1.5'>
              <Label htmlFor='failover-operation'>{t('Protocol')}</Label>
              <NativeSelect
                id='failover-operation'
                className='w-full'
                value={operation}
                onChange={(event) =>
                  setOperation(event.target.value as Operation)
                }
              >
                {(Object.keys(operationLabels) as Operation[]).map((item) => (
                  <NativeSelectOption key={item} value={item}>
                    {operationLabels[item]}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </div>
            <div className='grid gap-1.5'>
              <Label htmlFor='failover-minutes'>
                {t('Bypass duration (minutes)')}
              </Label>
              <Input
                id='failover-minutes'
                type='number'
                min={1}
                max={1440}
                required
                value={minutes}
                onChange={(event) => setMinutes(event.target.value)}
              />
            </div>
            <div className='grid gap-1.5'>
              <Label htmlFor='failover-reason'>{t('Reason')}</Label>
              <Input
                id='failover-reason'
                required
                maxLength={240}
                value={reason}
                onChange={(event) => setReason(event.target.value)}
              />
            </div>
          </div>
          <div className='flex flex-wrap gap-2'>
            <Button
              type='submit'
              disabled={mutation.isPending || !reason.trim()}
            >
              {t('Temporarily bypass')}
            </Button>
            <Button
              type='button'
              variant='outline'
              disabled={mutation.isPending}
              onClick={() => mutation.mutate('probe')}
            >
              {t('Check recovery now')}
            </Button>
            <Button
              type='button'
              variant='outline'
              disabled={mutation.isPending}
              onClick={() => mutation.mutate('permit_trial')}
            >
              {t('Permit controlled trial')}
            </Button>
            <Button
              type='button'
              variant='ghost'
              disabled={mutation.isPending}
              onClick={() => mutation.mutate('clear_bypass')}
            >
              {t('Clear temporary bypass')}
            </Button>
          </div>
        </form>
      ) : null}
    </section>
  )
}
