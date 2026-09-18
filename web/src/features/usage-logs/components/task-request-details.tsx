import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'

import { getTaskRequestBodies } from '../evidence-api'
import { EvidenceBodyStatus } from './evidence-body-status'

export function TaskRequestDetails(props: { taskId: string }) {
  const { t } = useTranslation()
  const [stage, setStage] = useState<
    'north_receive' | 'southbound_send' | null
  >(null)
  const query = useQuery({
    queryKey: ['task-request-bodies', props.taskId, stage],
    queryFn: () => getTaskRequestBodies(props.taskId, stage ?? 'north_receive'),
    enabled: stage !== null,
    gcTime: 0,
    retry: false,
  })
  return (
    <>
      <div className='flex flex-wrap gap-2'>
        <Button variant='outline' onClick={() => setStage('north_receive')}>
          {t('User request details')}
        </Button>
        <Button variant='outline' onClick={() => setStage('southbound_send')}>
          {t('Transformed upstream request')}
        </Button>
      </div>
      <Dialog
        open={stage !== null}
        onOpenChange={(open) => {
          if (!open) setStage(null)
        }}
        title={
          stage === 'southbound_send'
            ? t('Transformed upstream request')
            : t('User request details')
        }
        contentClassName='sm:max-w-4xl'
      >
        {query.isPending && <p role='status'>{t('Loading...')}</p>}
        {query.isError && (
          <div role='alert'>
            {t('Failed to load request evidence')}
            <Button variant='outline' onClick={() => void query.refetch()}>
              {t('Retry')}
            </Button>
          </div>
        )}
        {query.data?.length === 0 && <p>{t('No request evidence recorded')}</p>}
        <div className='space-y-3'>
          {query.data?.map((body) => (
            <section key={body.eventId} className='space-y-2'>
              {query.data.length > 1 && (
                <p className='text-xs break-all'>{body.requestId}</p>
              )}
              {!body.complete && <p>{t('Incomplete')}</p>}
              {body.text !== null ? (
                <pre className='max-h-[65vh] overflow-auto rounded-md border p-3 font-mono text-xs break-all whitespace-pre-wrap'>
                  {body.text}
                </pre>
              ) : (
                <EvidenceBodyStatus status={body.bodyStatus} />
              )}
            </section>
          ))}
        </div>
        {query.data?.some(
          (body) =>
            body.bodyStatus === 'read_failed' ||
            body.bodyStatus === 'storage_unavailable'
        ) && (
          <Button
            variant='outline'
            disabled={query.isFetching}
            onClick={() => void query.refetch()}
          >
            {t('Retry')}
          </Button>
        )}
      </Dialog>
    </>
  )
}
