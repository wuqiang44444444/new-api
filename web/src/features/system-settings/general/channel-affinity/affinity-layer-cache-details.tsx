import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toIntlLocale } from '@/i18n/languages'

import { getAffinityCacheEntries } from './api'
import type { AffinityCacheLayer } from './types'

export function AffinityLayerCacheDetails({
  layer,
  label,
}: {
  layer: AffinityCacheLayer
  label: string
}) {
  const { t, i18n } = useTranslation()
  const [open, setOpen] = useState(false)
  const [cursors, setCursors] = useState<string[]>([])
  const cursor = cursors.at(-1)
  const query = useQuery({
    queryKey: ['affinity-cache-entries', layer, cursor],
    queryFn: ({ signal }) => getAffinityCacheEntries(layer, cursor, signal),
    enabled: open,
    gcTime: 0,
    retry: false,
    refetchOnWindowFocus: false,
  })
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const nextCursor = query.data?.next_cursor
  let content
  if (query.isPending) {
    content = (
      <p role='status' className='text-muted-foreground'>
        {t('Loading...')}
      </p>
    )
  } else if (query.isError) {
    content = (
      <p role='alert' className='text-destructive'>
        {t('Loading failed')}
      </p>
    )
  } else if (query.data.entries.length === 0) {
    content = (
      <p role='status' className='text-muted-foreground'>
        {cursor || nextCursor
          ? t('No cache entries on this page')
          : t('No cache entries')}
      </p>
    )
  } else {
    content = (
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead scope='col'>{t('Key Fingerprint')}</TableHead>
            <TableHead scope='col'>{t('Channel')}</TableHead>
            <TableHead scope='col'>{t('Channel ID')}</TableHead>
            <TableHead scope='col'>{t('Remaining TTL (seconds)')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {query.data.entries.map((entry) => (
            <TableRow key={entry.fingerprint}>
              <TableCell>
                <code>{entry.fingerprint}</code>
              </TableCell>
              <TableCell className='max-w-52 break-all whitespace-normal'>
                {entry.channel_name || t('Unavailable')}
              </TableCell>
              <TableCell>{entry.channel_id}</TableCell>
              <TableCell>
                {entry.ttl_seconds === null
                  ? t('No expiry')
                  : entry.ttl_seconds.toLocaleString(locale)}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    )
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (nextOpen) setCursors([])
        setOpen(nextOpen)
      }}
      title={t('{{layer}} cache entries', { layer: label })}
      trigger={
        <Button
          type='button'
          size='sm'
          variant='outline'
          aria-label={t('View cache entries for {{layer}}', { layer: label })}
        >
          {t('View cache entries')}
        </Button>
      }
      footer={
        <>
          {(cursor || nextCursor) && (
            <>
              <Button
                variant='outline'
                disabled={!cursor || query.isFetching}
                onClick={() => setCursors((previous) => previous.slice(0, -1))}
              >
                {t('Previous page')}
              </Button>
              <Button
                variant='outline'
                disabled={!nextCursor || query.isFetching}
                onClick={() => {
                  if (nextCursor) {
                    setCursors((previous) => [...previous, nextCursor])
                  }
                }}
              >
                {t('Next page')}
              </Button>
            </>
          )}
          <Button
            variant='outline'
            disabled={query.isFetching}
            onClick={() => void query.refetch()}
          >
            {t('Refresh')}
          </Button>
        </>
      }
    >
      {content}
    </Dialog>
  )
}
