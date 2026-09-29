import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { readEvidenceOriginalText } from '../evidence-api'
import { EvidenceBodyStatus } from './evidence-body-status'

// Shared original-text dialog for the error log and task detail pages.
// The original body lives only in this dialog query: closing the dialog or
// unmounting (switching records) cancels in-flight reads and drops cached text.
export function EvidenceOriginalDialog(props: {
  id: number
  eventId: number
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const user = useAuthStore((state) => state.auth.user)
  const sessionId = useAuthStore((state) => state.auth.session?.sid)
  if (!props.open || user?.role !== ROLE.SUPER_ADMIN) return null
  return (
    <OriginalTextContent
      key={`${user.id}:${sessionId}:${props.id}:${props.eventId}`}
      viewerKey={`${user.id}:${sessionId}`}
      {...props}
    />
  )
}

function OriginalTextContent(props: {
  viewerKey: string
  id: number
  eventId: number
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { id, eventId, viewerKey } = props
  useEffect(
    () => () => {
      const queryKey = ['evidence-original', viewerKey, id, eventId]
      void queryClient.cancelQueries({ queryKey, exact: true })
      queryClient.removeQueries({ queryKey, exact: true })
    },
    [queryClient, viewerKey, id, eventId]
  )
  const query = useQuery({
    queryKey: ['evidence-original', viewerKey, id, eventId],
    queryFn: ({ signal }) => readEvidenceOriginalText(id, eventId, signal),
    gcTime: 0,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
  return (
    <Dialog
      open
      onOpenChange={props.onOpenChange}
      title={t('View original text')}
      description={t('Authentication credentials removed')}
      descriptionClassName='sr-only'
      contentClassName='sm:max-w-3xl'
    >
      <p className='text-muted-foreground text-xs'>
        {t('Authentication credentials removed')}
      </p>
      {query.isPending && <p role='status'>{t('Loading...')}</p>}
      {query.isError && (
        <div role='alert'>
          {t('Failed to load request evidence')}{' '}
          <Button variant='outline' onClick={() => void query.refetch()}>
            {t('Retry')}
          </Button>
        </div>
      )}
      {query.data && query.data.text !== null && (
        <pre className='max-h-[65vh] overflow-auto rounded-md border p-3 font-mono text-xs break-all whitespace-pre-wrap'>
          {query.data.text}
        </pre>
      )}
      {query.data && query.data.text === null && (
        <>
          <EvidenceBodyStatus status={query.data.bodyStatus} />
          {(query.data.bodyStatus === 'read_failed' ||
            query.data.bodyStatus === 'storage_unavailable') && (
            <Button
              variant='outline'
              disabled={query.isFetching}
              onClick={() => void query.refetch()}
            >
              {t('Retry')}
            </Button>
          )}
        </>
      )}
    </Dialog>
  )
}
