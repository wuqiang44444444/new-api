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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import { postAdminUpstreamDiscountInit, putAdminUpstreamDiscount } from '../api'
import type { ProviderChannelDiscountStatus } from '../types'
import {
  upstreamDiscountLabel,
  upstreamDiscountSourceLabel,
} from '../upstream-reconciliation-utils'

type UpstreamDiscountEditorProps = {
  periodStart: number
  month: string
  urlKey: string
  discounts: ProviderChannelDiscountStatus[]
  onChanged: () => void
}

type DraftState = {
  channelId: number
  expectedVersion: number
  percent: string
  reason: string
}

// 渠道月度折扣编辑区：一个渠道当月一个综合系数；输入按百分比（80 → 0.8），
// 避免把 8 误输成系数。保存携带 expected_version，冲突时提示重新读取。
export function UpstreamDiscountEditor(props: UpstreamDiscountEditorProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [draft, setDraft] = useState<DraftState | null>(null)

  const invalidate = () => {
    queryClient.invalidateQueries({
      queryKey: ['billing-upstream-reconciliation'],
    })
    props.onChanged()
  }

  const saveMutation = useMutation({
    mutationFn: async (payload: {
      period_start: number
      channel_id: number
      discount: string
      expected_version: number
      reason: string
    }) => putAdminUpstreamDiscount(payload),
    onSuccess: (response) => {
      if (!response.success) {
        toast.error(response.message || t('Unable to save the discount.'))
        return
      }
      toast.success(t('Channel discount saved.'))
      setDraft(null)
      invalidate()
    },
    onError: (error: unknown) => {
      const message = error instanceof Error ? error.message : ''
      if (message.includes('409') || message.includes('conflict')) {
        toast.error(
          t('The discount was changed by someone else. Refresh and try again.')
        )
      } else {
        toast.error(message || t('Unable to save the discount.'))
      }
    },
  })

  const initMutation = useMutation({
    mutationFn: async () =>
      postAdminUpstreamDiscountInit({
        period_start: props.periodStart,
        channel_ids: props.discounts.map((status) => status.channel_id),
      }),
    onSuccess: (response) => {
      if (!response.success) {
        toast.error(
          response.message || t('Unable to initialize monthly discounts.')
        )
        return
      }
      const created = response.data?.outcomes.filter(
        (outcome) => outcome.outcome === 'created'
      ).length
      const missing = response.data?.outcomes.filter(
        (outcome) => outcome.outcome === 'defaulted'
      ).length
      if (created) {
        toast.success(
          t('Inherited {{count}} channel discounts from last month.', {
            count: created,
          })
        )
      }
      if (missing) {
        toast.info(
          t('{{count}} channels use the default coefficient 1.', {
            count: missing,
          })
        )
      }
      if (!created && !missing) {
        toast.info(t('Monthly discounts are already initialized.'))
      }
      invalidate()
    },
    onError: () => {
      toast.error(t('Unable to initialize monthly discounts.'))
    },
  })

  const startEdit = (status: ProviderChannelDiscountStatus) => {
    const percent = status.discount
      ? String(Number(status.discount.value) * 100)
      : ''
    setDraft({
      channelId: status.channel_id,
      expectedVersion: status.discount?.version ?? 0,
      percent,
      reason: '',
    })
  }

  const submitDraft = () => {
    if (!draft) return
    const percent = Number(draft.percent)
    if (!Number.isFinite(percent) || percent <= 0 || percent > 100) {
      toast.error(t('Enter a discount between 0% and 100% (exclusive of 0).'))
      return
    }
    if (!draft.reason.trim()) {
      toast.error(t('A reason is required to change a discount.'))
      return
    }
    const coefficient = String(Number((percent / 100).toFixed(8)))
    saveMutation.mutate({
      period_start: props.periodStart,
      channel_id: draft.channelId,
      discount: coefficient,
      expected_version: draft.expectedVersion,
      reason: draft.reason.trim(),
    })
  }

  const hasPending = props.discounts.some((status) => !status.discount)

  return (
    <div className='space-y-2'>
      <div className='flex items-center justify-between gap-2'>
        <p className='text-muted-foreground text-xs'>
          {t(
            'One comprehensive coefficient per channel per month; it applies to every model and billing mode and never changes customer charges.'
          )}
        </p>
        <Button
          variant='outline'
          size='xs'
          disabled={initMutation.isPending}
          onClick={() => initMutation.mutate()}
        >
          {t('Initialize from last month')}
        </Button>
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Upstream channel')}</TableHead>
            <TableHead>{t('Current coefficient')}</TableHead>
            <TableHead>{t('Source')}</TableHead>
            <TableHead className='text-right'>{t('Actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.discounts.map((status) => (
            <TableRow key={status.channel_id}>
              <TableCell>
                <div className='flex flex-col'>
                  <span>{status.channel_name}</span>
                  <span className='text-muted-foreground text-xs'>
                    #{status.channel_id}
                  </span>
                </div>
              </TableCell>
              <TableCell>
                {status.discount ? (
                  <div className='flex flex-col'>
                    <span>{upstreamDiscountLabel(status, t)}</span>
                    <span className='text-muted-foreground text-xs'>
                      {t('v{{version}} · updated {{time}}', {
                        version: status.discount.version,
                        time: status.updated_at
                          ? new Date(status.updated_at * 1000).toLocaleString()
                          : '—',
                      })}
                    </span>
                  </div>
                ) : (
                  <span className='text-amber-600 dark:text-amber-400'>
                    {t('Pending manual fill')}
                  </span>
                )}
              </TableCell>
              <TableCell className='text-muted-foreground text-xs'>
                {upstreamDiscountSourceLabel(status, t)}
              </TableCell>
              <TableCell className='text-right'>
                {draft?.channelId === status.channel_id ? (
                  <div className='flex flex-col items-stretch gap-1 sm:flex-row sm:items-center sm:justify-end'>
                    <Input
                      aria-label={t('Discount percent')}
                      className='w-24'
                      inputMode='decimal'
                      placeholder={t('Percent, e.g. 80')}
                      value={draft.percent}
                      onChange={(event) =>
                        setDraft({ ...draft, percent: event.target.value })
                      }
                    />
                    <Input
                      aria-label={t('Change reason')}
                      className='w-56'
                      placeholder={t('Reason')}
                      value={draft.reason}
                      onChange={(event) =>
                        setDraft({ ...draft, reason: event.target.value })
                      }
                    />
                    <Button
                      size='xs'
                      disabled={saveMutation.isPending}
                      onClick={submitDraft}
                    >
                      {t('Save')}
                    </Button>
                    <Button
                      variant='ghost'
                      size='xs'
                      onClick={() => setDraft(null)}
                    >
                      {t('Cancel')}
                    </Button>
                  </div>
                ) : (
                  <Button
                    variant='link'
                    size='xs'
                    onClick={() => startEdit(status)}
                  >
                    {status.discount ? t('Edit') : t('Fill in')}
                  </Button>
                )}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      {hasPending ? (
        <p className='text-muted-foreground text-xs'>
          {t(
            'Historical conflicts require confirmation. Unconfigured channels use the default coefficient 1.'
          )}
        </p>
      ) : null}
    </div>
  )
}
