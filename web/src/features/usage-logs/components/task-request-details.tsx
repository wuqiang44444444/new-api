import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { getTaskRequestBodies } from '../evidence-api'
import { EvidenceBodyStatus } from './evidence-body-status'

export function TaskRequestDetails(props: { taskId: string }) {
  const user = useAuthStore((state) => state.auth.user)
  const sessionId = useAuthStore((state) => state.auth.session?.sid)
  if (user?.role !== ROLE.SUPER_ADMIN) return null
  return (
    <TaskRequestShortcuts
      key={`${user.id}:${sessionId}:${props.taskId}`}
      taskId={props.taskId}
      viewerKey={`${user.id}:${sessionId}`}
    />
  )
}

function TaskRequestShortcuts(props: { taskId: string; viewerKey: string }) {
  const { t } = useTranslation()
  const [stage, setStage] = useState<
    'north_receive' | 'southbound_send' | null
  >(null)
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
      {stage !== null && (
        <TaskRequestBodyDialog
          viewerKey={props.viewerKey}
          key={stage}
          taskId={props.taskId}
          stage={stage}
          onClose={() => setStage(null)}
        />
      )}
    </>
  )
}

function TaskRequestBodyDialog(props: {
  viewerKey: string
  taskId: string
  stage: 'north_receive' | 'southbound_send'
  onClose: () => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { taskId, stage, viewerKey } = props
  useEffect(
    () => () => {
      const queryKey = ['task-request-bodies', viewerKey, taskId, stage]
      void queryClient.cancelQueries({ queryKey, exact: true })
      queryClient.removeQueries({ queryKey, exact: true })
    },
    [queryClient, viewerKey, taskId, stage]
  )
  const query = useQuery({
    queryKey: ['task-request-bodies', viewerKey, taskId, stage],
    queryFn: ({ signal }) => getTaskRequestBodies(taskId, stage, signal),
    gcTime: 0,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose()
      }}
      title={
        stage === 'southbound_send'
          ? t('Transformed upstream request')
          : t('User request details')
      }
      description={t('Authentication credentials removed')}
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
  )
}
