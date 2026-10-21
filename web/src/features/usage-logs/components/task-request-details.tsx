import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { getTaskRequestEntries } from '../evidence-api'
import { EvidenceBodyStatus } from './evidence-body-status'
import { EvidenceOriginalDialog } from './evidence-original-dialog'

export function TaskRequestDetails(props: { taskId: string }) {
  const user = useAuthStore((state) => state.auth.user)
  const sessionId = useAuthStore((state) => state.auth.session?.sid)
  if (!user || user.role < ROLE.ADMIN) return null
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
  const [contentName, setContentName] = useState<
    'north_receive' | 'southbound_send' | null
  >(null)
  return (
    <>
      <div className='flex flex-wrap gap-2'>
        <Button
          variant='outline'
          onClick={() => setContentName('north_receive')}
        >
          {t('User request details')}
        </Button>
        <Button
          variant='outline'
          onClick={() => setContentName('southbound_send')}
        >
          {t('Transformed upstream request')}
        </Button>
      </div>
      {contentName !== null && (
        <TaskRequestBodyDialog
          viewerKey={props.viewerKey}
          key={contentName}
          taskId={props.taskId}
          contentName={contentName}
          onClose={() => setContentName(null)}
        />
      )}
    </>
  )
}

function TaskRequestBodyDialog(props: {
  viewerKey: string
  taskId: string
  contentName: 'north_receive' | 'southbound_send'
  onClose: () => void
}) {
  const { t } = useTranslation()
  const [selected, setSelected] = useState<{
    evidenceId: number
    eventId: number
  } | null>(null)
  const queryClient = useQueryClient()
  const { taskId, contentName, viewerKey } = props
  useEffect(
    () => () => {
      const queryKey = ['task-request-entries', viewerKey, taskId, contentName]
      void queryClient.cancelQueries({ queryKey, exact: true })
      queryClient.removeQueries({ queryKey, exact: true })
    },
    [queryClient, viewerKey, taskId, contentName]
  )
  const query = useQuery({
    queryKey: ['task-request-entries', viewerKey, taskId, contentName],
    queryFn: ({ signal }) => getTaskRequestEntries(taskId, contentName, signal),
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
        contentName === 'southbound_send'
          ? t('Transformed upstream request')
          : t('User request details')
      }
      description={t(
        'Captured content may be incomplete and may contain credentials.'
      )}
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
      {query.data?.length === 0 && (
        <p>
          {t(
            'No request evidence available. It may not have been recorded or may have expired.'
          )}
        </p>
      )}
      <div className='space-y-3'>
        {query.data?.map((body) => (
          <section key={body.eventId} className='space-y-2'>
            {query.data.length > 1 && (
              <p className='text-xs break-all'>{body.requestId}</p>
            )}
            {body.expired ? (
              <EvidenceBodyStatus status='expired' />
            ) : (
              <Button variant='outline' onClick={() => setSelected(body)}>
                {t('View original text')} · {body.eventId}
              </Button>
            )}
          </section>
        ))}
      </div>
      {selected && (
        <EvidenceOriginalDialog
          id={selected.evidenceId}
          eventId={selected.eventId}
          open
          onOpenChange={(open) => {
            if (!open) setSelected(null)
          }}
        />
      )}
    </Dialog>
  )
}
