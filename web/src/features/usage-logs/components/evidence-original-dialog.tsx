import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { downloadEvidence, readEvidenceOriginalText } from '../evidence-api'
import { EvidenceBodyStatus } from './evidence-body-status'
import { EvidenceContentPreview } from './evidence-content-preview'

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
  if (!props.open || !user || user.role < ROLE.ADMIN) return null
  return (
    <OriginalTextContent
      key={`${user.id}:${user.role}:${sessionId}:${props.id}:${props.eventId}`}
      viewerKey={`${user.id}:${user.role}:${sessionId}`}
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
  const [downloadError, setDownloadError] = useState(false)
  const [formatJSON, setFormatJSON] = useState(false)
  const queryClient = useQueryClient()
  const { id, eventId, viewerKey } = props
  useEffect(
    () => () => {
      const queryKey = ['evidence-original', viewerKey, id, eventId]
      void queryClient.cancelQueries({ queryKey })
      queryClient.removeQueries({ queryKey })
    },
    [queryClient, viewerKey, id, eventId]
  )
  const query = useQuery({
    queryKey: ['evidence-original', viewerKey, id, eventId, formatJSON],
    queryFn: ({ signal }) =>
      readEvidenceOriginalText(id, eventId, signal, formatJSON),
    placeholderData: (previous) => previous,
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
      description={t(
        'Captured content may be incomplete and may contain credentials.'
      )}
      descriptionClassName='sr-only'
      contentClassName='sm:max-w-3xl'
    >
      <p className='text-muted-foreground text-xs'>
        {t('Captured content may be incomplete and may contain credentials.')}
      </p>
      {query.isPending && <p role='status'>{t('Loading...')}</p>}
      {query.isError && (
        <div role='alert'>
          {t('Failed to load request evidence')}{' '}
          <Button variant='outline' onClick={() => void query.refetch()}>
            {t('Retry')}
          </Button>
          {formatJSON && (
            <Button variant='outline' onClick={() => setFormatJSON(false)}>
              {t('Show original text')}
            </Button>
          )}
        </div>
      )}
      {query.data?.bodyStatus === 'available' && (
        <>
          <div className='flex flex-wrap gap-2'>
            <Button
              variant='outline'
              aria-pressed={formatJSON}
              aria-busy={query.isFetching}
              disabled={
                query.isFetching ||
                (!formatJSON &&
                  !query.data.messages &&
                  (query.data.display_limited ||
                    typeof query.data.text !== 'string'))
              }
              onClick={() => setFormatJSON(!formatJSON)}
            >
              {formatJSON ? t('Show original text') : t('Format JSON')}
            </Button>
            <Button
              variant='outline'
              onClick={() => {
                setDownloadError(false)
                void downloadEvidence(id, eventId).catch(() =>
                  setDownloadError(true)
                )
              }}
            >
              {t('Download original evidence')}
            </Button>
          </div>
          {query.data.display_limited && (
            <p role='status' className='text-muted-foreground text-sm'>
              {t(
                'Only the first 64 KiB is shown. Download the full evidence for the complete captured content.'
              )}
            </p>
          )}
          {downloadError && (
            <p role='alert'>{t('Failed to download evidence')}</p>
          )}
          {query.data.meta && (
            <details>
              <summary>{t('Additional information')}</summary>
              <pre className='max-h-48 overflow-auto break-all whitespace-pre-wrap'>
                {JSON.stringify(query.data.meta)}
              </pre>
            </details>
          )}
          {query.isFetching && <p role='status'>{t('Loading...')}</p>}
          <EvidenceContentPreview content={query.data} formatted={formatJSON} />
        </>
      )}
      {query.data && query.data.bodyStatus !== 'available' && (
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
