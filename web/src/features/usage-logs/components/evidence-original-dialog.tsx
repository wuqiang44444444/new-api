import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { downloadEvidence, readEvidenceOriginalText } from '../evidence-api'
import { formatEvidenceJSON } from '../lib/format-evidence-json'
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
  const [formatted, setFormatted] = useState<string | null>(null)
  const [formatError, setFormatError] = useState(false)
  const [downloadError, setDownloadError] = useState(false)
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
  const rawText = query.data?.text
  const imageType = query.data?.content_type
    ?.split(';', 1)[0]
    ?.trim()
    .toLowerCase()
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
        </div>
      )}
      {query.data?.bodyStatus === 'available' && (
        <>
          <div className='flex flex-wrap gap-2'>
            <Button
              variant='outline'
              onClick={() => {
                setFormatted(null)
                setFormatError(false)
              }}
            >
              {t('Raw')}
            </Button>
            {typeof rawText === 'string' && (
              <Button
                variant='outline'
                onClick={() => {
                  try {
                    setFormatted(formatEvidenceJSON(rawText))
                    setFormatError(false)
                  } catch {
                    setFormatted(null)
                    setFormatError(true)
                  }
                }}
              >
                {t('Format JSON')}
              </Button>
            )}
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
          {formatError && (
            <p role='status'>
              {t('Unable to format JSON. Showing raw content.')}
            </p>
          )}
          {downloadError && (
            <p role='alert'>{t('Failed to download evidence')}</p>
          )}
          {query.data.meta && (
            <details>
              <summary>{t('Additional information')}</summary>
              <pre className='max-h-48 overflow-auto break-all whitespace-pre-wrap'>
                {JSON.stringify(query.data.meta, null, 2)}
              </pre>
            </details>
          )}
          {typeof rawText === 'string' && (
            <pre className='max-h-[65vh] overflow-auto rounded-md border p-3 font-mono text-xs break-all whitespace-pre-wrap'>
              {formatted ?? rawText}
            </pre>
          )}
          {query.data.base64 && (
            <pre
              aria-label='Base64'
              className='max-h-48 overflow-auto break-all whitespace-pre-wrap'
            >
              {query.data.base64}
            </pre>
          )}
          {query.data.base64 &&
            /^(image\/(png|jpeg|gif|webp|avif))$/i.test(imageType ?? '') && (
              <img
                className='max-h-[65vh] max-w-full object-contain'
                src={`data:${imageType};base64,${query.data.base64}`}
                alt={t('Evidence image preview')}
              />
            )}
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
