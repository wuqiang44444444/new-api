import { useMutation, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import {
  downloadEvidence,
  getEvidenceDetail,
  getEvidenceList,
} from '../evidence-api'

export function TaskEvidence(props: {
  taskId?: string
  requestId?: string
  isRoot: boolean
}) {
  const { t } = useTranslation()
  const [opened, setOpened] = useState(false)
  const [page, setPage] = useState(1)
  const query = useQuery({
    queryKey: [
      'task-evidence',
      props.taskId,
      props.requestId,
      page,
      props.isRoot,
    ],
    queryFn: () =>
      getEvidenceList(
        { task_id: props.taskId, request_id: props.requestId },
        page
      ),
    enabled: opened && Boolean(props.taskId || props.requestId),
    gcTime: 0,
    retry: false,
  })
  return (
    <section className='space-y-2'>
      <Button
        variant='outline'
        aria-expanded={opened}
        onClick={() => setOpened(!opened)}
      >
        {t('Request evidence')}
      </Button>
      {opened && (
        <div className='space-y-2'>
          {query.isPending && <p role='status'>{t('Loading...')}</p>}
          {query.isError && (
            <div role='alert'>
              {t('Failed to load request evidence')}{' '}
              <Button variant='outline' onClick={() => void query.refetch()}>
                {t('Retry')}
              </Button>
            </div>
          )}
          {query.data?.items.length === 0 && (
            <p>{t('No request evidence recorded')}</p>
          )}
          {query.data?.items.map((item) => (
            <EvidenceDetails
              key={item.id}
              id={item.id}
              label={item.request_id || String(item.id)}
              isRoot={props.isRoot}
            />
          ))}
          {query.data && query.data.total > 20 && (
            <div className='flex gap-2'>
              <Button disabled={page === 1} onClick={() => setPage(page - 1)}>
                {t('Previous')}
              </Button>
              <Button
                disabled={page * 20 >= query.data.total}
                onClick={() => setPage(page + 1)}
              >
                {t('Next')}
              </Button>
            </div>
          )}
        </div>
      )}
    </section>
  )
}

function EvidenceDetails(props: {
  id: number
  label: string
  isRoot: boolean
}) {
  const { t } = useTranslation()
  const [opened, setOpened] = useState(false)
  const query = useQuery({
    queryKey: ['task-evidence-detail', props.id, props.isRoot],
    queryFn: () => getEvidenceDetail(props.id),
    enabled: opened,
    gcTime: 0,
    retry: false,
  })
  const download = useMutation({
    mutationFn: (eventId: number) => downloadEvidence(props.id, eventId),
  })
  const stages: Record<string, string> = {
    north_receive: t('Client request'),
    southbound_send: t('Upstream request'),
    upstream_response: t('Upstream response'),
    polling: t('Task polling'),
    transport: t('Failed'),
    client_delivery: t('Result delivery'),
  }
  return (
    <div className='min-w-0 rounded-md border p-2'>
      <Button
        variant='ghost'
        className='max-w-full break-all whitespace-normal'
        aria-expanded={opened}
        onClick={() => setOpened(!opened)}
      >
        {props.label}
      </Button>
      {opened && (
        <div className='space-y-2'>
          {query.isPending && <p role='status'>{t('Loading...')}</p>}
          {query.isError && (
            <div role='alert'>
              {t('Failed to load request evidence')}{' '}
              <Button onClick={() => void query.refetch()}>{t('Retry')}</Button>
            </div>
          )}
          {query.data?.evidence.body_expired && (
            <p>{t('Evidence body expired')}</p>
          )}
          {query.data?.events.map((event) => (
            <article
              key={event.id}
              className='min-w-0 space-y-1 rounded-md border p-2 text-xs'
            >
              <p>
                {stages[event.stage] || event.stage} ·{' '}
                {event.complete ? t('Complete') : t('Incomplete')}
              </p>
              <p>
                {t('Status')}: {event.phase} · HTTP {event.status_code || '-'} ·{' '}
                {event.byte_count} B
              </p>
              {event.preview && (
                <pre className='max-h-64 overflow-auto break-all whitespace-pre-wrap'>
                  {event.preview}
                </pre>
              )}
              {!event.has_body && <p>{t('Evidence body unavailable')}</p>}
              {props.isRoot &&
                event.has_body &&
                !query.data?.evidence.body_expired && (
                  <Button
                    variant='outline'
                    disabled={download.isPending}
                    onClick={() => download.mutate(event.id)}
                  >
                    {t('Download original evidence')}
                  </Button>
                )}
            </article>
          ))}
          {download.isError && (
            <p role='alert'>{t('Failed to download evidence')}</p>
          )}
        </div>
      )}
    </div>
  )
}
