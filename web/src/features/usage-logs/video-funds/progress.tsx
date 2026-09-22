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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

import { getSelfVideoFunds } from './api'

export function VideoFundProgressPanel() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [taskId, setTaskId] = useState('')
  const query = useQuery({
    queryKey: ['self-video-funds', page, taskId],
    queryFn: () => getSelfVideoFunds(page, taskId),
    retry: false,
    refetchInterval: 15000,
  })
  const state = (value: string) => {
    if (value === 'held') return t('Funds held')
    if (value === 'pending') return t('Refund pending')
    if (value === 'refunded') return t('Refunded')
    if (value === 'charged') return t('Charged')
    return t('No current charge')
  }
  const date = (value: number) =>
    value > 0 ? new Date(value * 1000).toLocaleString() : '—'
  return (
    <section
      className='flex h-full flex-col gap-3 overflow-auto'
      aria-label={t('Video refund progress')}
    >
      <p className='text-muted-foreground text-sm'>
        {t(
          'Creation holds are released after 24 hours. Do not automatically resubmit an unknown request.'
        )}
      </p>
      <Input
        aria-label={t('Task ID')}
        placeholder={t('Task ID')}
        value={taskId}
        onChange={(e) => {
          setTaskId(e.target.value)
          setPage(1)
        }}
      />
      {query.isPending && <p role='status'>{t('Loading...')}</p>}
      {query.isError && (
        <p role='alert'>
          {t('Failed to load video fund records')}{' '}
          <Button variant='outline' onClick={() => void query.refetch()}>
            {t('Retry')}
          </Button>
        </p>
      )}
      {!query.isError &&
        query.data?.items.map((item) => (
          <article key={item.task_id} className='rounded-md border p-3 text-sm'>
            <p className='font-medium break-all'>
              {item.task_id} · {item.model}
            </p>
            <dl className='grid grid-cols-2 gap-2'>
              <dt>{t('Funding status')}</dt>
              <dd>{state(item.fund_state)}</dd>
              <dt>{t('Current amount (quota)')}</dt>
              <dd>{item.quota.toLocaleString()}</dd>
              <dt>{t('Returned amount (quota)')}</dt>
              <dd>
                {item.refund_amount_known
                  ? item.refunded_quota.toLocaleString()
                  : t('Unknown')}
              </dd>
              <dt>{t('Automatic refund deadline')}</dt>
              <dd>{date(item.deadline_at)}</dd>
              <dt>{t('Refund completed at')}</dt>
              <dd>{date(item.refunded_at)}</dd>
              <dt>{t('Next refund retry')}</dt>
              <dd>{date(item.retry_at)}</dd>
            </dl>
          </article>
        ))}
      {!query.isError && query.data?.total === 0 && <p>{t('No records')}</p>}
      <div className='flex gap-2'>
        <Button
          variant='outline'
          disabled={page <= 1}
          onClick={() => setPage(page - 1)}
        >
          {t('Previous')}
        </Button>
        <Button
          variant='outline'
          disabled={query.isFetching || page * 20 >= (query.data?.total ?? 0)}
          onClick={() => setPage(page + 1)}
        >
          {t('Next')}
        </Button>
      </div>
    </section>
  )
}
