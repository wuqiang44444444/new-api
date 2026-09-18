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
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import dayjs from '@/lib/dayjs'

import {
  DetailRow,
  DetailSection,
} from '../../components/dialogs/log-detail-layout'
import type { ErrorLogItem } from '../api'
import { errorEventTypeLabel } from './error-event-type'
import { ErrorLogHTTPExchange } from './error-log-http-exchange'
import { ErrorLogStatus } from './error-log-status'

function formatChannelLabel(entry: ErrorLogItem): string {
  if (entry.channel_name) return `${entry.channel_name} (#${entry.channel_id})`
  if (entry.channel_id) return `#${entry.channel_id}`
  return '—'
}

export function ErrorLogDetailsDialog(props: { entry: ErrorLogItem }) {
  const { t } = useTranslation()
  const detailEntries = useMemo(() => {
    if (!props.entry.detail) return []
    try {
      const parsed: unknown = JSON.parse(props.entry.detail)
      if (!parsed || typeof parsed !== 'object') return []
      return Object.entries(parsed as Record<string, string>).filter(
        ([key, value]) => key !== 'http_exchange' && typeof value === 'string'
      )
    } catch {
      return []
    }
  }, [props.entry.detail])
  const channelValue = formatChannelLabel(props.entry)
  return (
    <Dialog
      title={t('Log Details')}
      description={t('View the complete details for this log entry')}
      descriptionClassName='sr-only'
      trigger={
        <Button variant='ghost' size='sm' className='h-7 px-2'>
          {t('Details')}
        </Button>
      }
      contentClassName='min-w-0 sm:max-w-lg max-sm:max-h-[calc(100dvh-1.5rem)] max-sm:w-[calc(100vw-1.5rem)] max-sm:max-w-[calc(100vw-1.5rem)]'
      titleClassName='text-base'
      contentHeight='auto'
      bodyClassName='space-y-3'
    >
      <div className='min-w-0 space-y-1.5'>
        <div className='flex flex-wrap items-center gap-2 text-xs'>
          <ErrorLogStatus entry={props.entry} />
          {props.entry.event_type === 'channel_test' &&
            props.entry.status > 0 && (
              <span className='text-muted-foreground'>
                {t('Management HTTP {{status}}', {
                  status: props.entry.status,
                })}
              </span>
            )}
          <span className='text-muted-foreground font-mono'>
            {props.entry.method} {props.entry.route || '—'}
          </span>
          {Number.isFinite(props.entry.created_at) && (
            <span className='text-muted-foreground tabular-nums'>
              {dayjs.unix(props.entry.created_at).format('YYYY-MM-DD HH:mm:ss')}
            </span>
          )}
        </div>
      </div>
      <DetailSection label={t('Error information')}>
        <DetailRow
          label={t('Event Type')}
          value={errorEventTypeLabel(props.entry.event_type, t)}
        />
        <DetailRow
          label={t('Reason')}
          value={
            [props.entry.reason, props.entry.public_code]
              .filter(Boolean)
              .join(' · ') || '—'
          }
          mono
        />
        {!!props.entry.stage && (
          <DetailRow label={t('Stage')} value={props.entry.stage} mono />
        )}
        {!!props.entry.protocol && (
          <DetailRow label={t('Protocol')} value={props.entry.protocol} mono />
        )}
        {!!props.entry.elapsed_ms && (
          <DetailRow
            label={t('Elapsed')}
            value={`${(props.entry.elapsed_ms / 1000).toFixed(2)}s`}
            mono
          />
        )}
      </DetailSection>
      <DetailSection label={t('Caller')}>
        <DetailRow
          label={t('Username')}
          value={
            props.entry.username
              ? `${props.entry.username} (#${props.entry.user_id})`
              : '—'
          }
        />
        <DetailRow
          label={t('Token Name')}
          value={props.entry.token_name || '—'}
        />
        <DetailRow
          label={t('Model')}
          value={props.entry.model_name || '—'}
          mono
        />
        <DetailRow label={t('Channel')} value={channelValue} />
      </DetailSection>
      <DetailSection label={t('Request identification')}>
        <DetailRow
          label={t('Request ID')}
          value={props.entry.request_id || '—'}
          mono
        />
        {!!props.entry.task_id && (
          <DetailRow label={t('Task ID')} value={props.entry.task_id} mono />
        )}
        <DetailRow
          label={t('Upstream Request ID')}
          value={props.entry.upstream_request_id || '—'}
          mono
        />
      </DetailSection>
      <ErrorLogHTTPExchange detail={props.entry.detail} />
      {detailEntries.length > 0 && (
        <DetailSection label={t('Additional information')}>
          {detailEntries.map(([key, value]) => (
            <DetailRow key={key} label={key} value={value} mono />
          ))}
        </DetailSection>
      )}
    </Dialog>
  )
}
