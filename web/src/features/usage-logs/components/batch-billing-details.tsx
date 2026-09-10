import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { api } from '@/lib/api'

interface BatchBilling {
  status: string
  delivery_state: string
  settle_state: string
  request_count: number
  estimated_quota: number
  target_quota: number | null
  has_more: boolean
  lines: {
    custom_id: string
    status: string
    input_tokens: number
    output_tokens: number
    cached_tokens: number
    quota: number
  }[]
}

export function BatchBillingDetails({ id }: { id: string }) {
  const { t } = useTranslation()
  const statuses: Record<string, string> = {
    validating: t('Validating'),
    in_progress: t('Processing...'),
    finalizing: t('Finalizing'),
    cancelling: t('Cancelling'),
    cancelled: t('Cancelled'),
    completed: t('Completed'),
    failed: t('Failed'),
    expired: t('Expired'),
    pending: t('Pending'),
    processing: t('Processing...'),
    ready: t('Ready'),
    settled: t('Settled'),
    debt: t('Insufficient balance'),
  }
  const [offset, setOffset] = useState(0)
  const query = useQuery({
    queryKey: ['batch-billing', id, offset],
    queryFn: async () => {
      const response = await api.get<{ success: boolean; data: BatchBilling }>(
        `/api/batch/${encodeURIComponent(id)}/billing`,
        { params: { offset } }
      )
      if (!response.data.success) throw new Error('Batch billing unavailable')
      return response.data.data
    },
  })
  if (query.isPending) return <p>{t('Loading...')}</p>
  if (query.isError) {
    return (
      <div role='alert'>
        {t('Loading failed')}{' '}
        <Button onClick={() => query.refetch()}>{t('Retry')}</Button>
      </div>
    )
  }
  const data = query.data
  return (
    <section className='space-y-3' aria-label={t('Batch billing')}>
      <h3 className='font-medium'>{t('Batch billing')}</h3>
      <p>
        {t('Status')}: {statuses[data.status] ?? t('Unknown')} ·{' '}
        {t('Settlement')}: {statuses[data.settle_state] ?? t('Unknown')} ·{' '}
        {t('Delivery')}: {statuses[data.delivery_state] ?? t('Unknown')}
      </p>
      <p>
        {t('Requests')}: {data.request_count} · {t('Quota')}:{' '}
        {data.target_quota ?? t('Pending')}
      </p>
      <div className='overflow-x-auto'>
        <table className='w-full text-sm'>
          <thead>
            <tr>
              {[
                'Request ID',
                'Status',
                'Input Tokens',
                'Output Tokens',
                'Cached Tokens',
                'Quota',
              ].map((label) => (
                <th key={label} className='p-2 text-left'>
                  {t(label)}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {data.lines.map((line) => (
              <tr key={line.custom_id}>
                <td className='max-w-48 truncate p-2'>{line.custom_id}</td>
                <td className='p-2'>{statuses[line.status] ?? t('Unknown')}</td>
                <td className='p-2'>{line.input_tokens}</td>
                <td className='p-2'>{line.output_tokens}</td>
                <td className='p-2'>{line.cached_tokens}</td>
                <td className='p-2'>{line.quota}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <div className='flex gap-2'>
        <Button
          disabled={offset === 0}
          onClick={() => setOffset(Math.max(0, offset - 100))}
        >
          {t('Previous')}
        </Button>
        <Button
          disabled={!data.has_more}
          onClick={() => setOffset(offset + 100)}
        >
          {t('Next')}
        </Button>
      </div>
    </section>
  )
}
