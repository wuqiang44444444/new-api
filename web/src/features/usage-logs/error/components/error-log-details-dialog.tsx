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
import { Link } from '@tanstack/react-router'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { AUTO_CHECK_DETAIL_KEYS } from '@/features/channels/components/channel-check-detail-keys'
import { ChannelCheckDetails } from '@/features/channels/components/channel-check-details'
import dayjs from '@/lib/dayjs'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import {
  DetailRow,
  DetailSection,
} from '../../components/dialogs/log-detail-layout'
import { TaskEvidence } from '../../components/task-evidence'
import type { ErrorLogItem } from '../api'
import { mediaAutoProbeDetail } from './auto-probe-detail'
import { AutomaticProbeLabel } from './automatic-probe-label'
import { ErrorLogHTTPExchange } from './error-log-http-exchange'
import { ErrorLogStatus } from './error-log-status'

function formatChannelLabel(entry: ErrorLogItem): string {
  if (entry.channel_name) return `${entry.channel_name} (#${entry.channel_id})`
  if (entry.channel_id) return `#${entry.channel_id}`
  return '—'
}

export function ErrorLogDetailsDialog(props: { entry: ErrorLogItem }) {
  const { t } = useTranslation()
  const isRoot = useAuthStore(
    (state) => state.auth.user?.role === ROLE.SUPER_ADMIN
  )
  const detail = useMemo((): Record<string, string> => {
    try {
      const parsed: unknown = JSON.parse(props.entry.detail || '{}')
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        return {}
      }
      return Object.fromEntries(
        Object.entries(parsed).filter(
          (entry): entry is [string, string] => typeof entry[1] === 'string'
        )
      )
    } catch {
      return {}
    }
  }, [props.entry.detail])
  const isTaskFailure = props.entry.event_type === 'task_failure'
  const isAutoChannelTest =
    props.entry.event_type === 'channel_test' && detail.test_mode === 'auto'
  // A linked identity permits a lookup; only recorded evidence determines
  // whether the original is available, including events with HTTP snapshots.
  const showTaskEvidence = Boolean(
    props.entry.task_id || props.entry.request_id
  )
  const detailEntries = Object.entries(detail).filter(
    ([key]) =>
      key !== 'http_exchange' &&
      !(isTaskFailure && key === 'fail_reason') &&
      !(isTaskFailure && key === 'create_upstream_request_id') &&
      !(
        isAutoChannelTest &&
        (AUTO_CHECK_DETAIL_KEYS as readonly string[]).includes(key)
      )
  )
  const autoCheckEntries = isAutoChannelTest
    ? AUTO_CHECK_DETAIL_KEYS.filter((key) => detail[key]).map(
        (key) => [key, detail[key]] as const
      )
    : []
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
      {mediaAutoProbeDetail(props.entry) && (
        <p className='bg-muted text-muted-foreground rounded-md p-3 text-sm'>
          {t(
            'This is an automatic probe, not a customer request. It does not establish whether image or video generation succeeds.'
          )}
        </p>
      )}
      <DetailSection label={t('Error information')}>
        <DetailRow
          label={t('Event Type')}
          value={<AutomaticProbeLabel entry={props.entry} />}
        />
        <DetailRow
          label={t('Reason')}
          value={
            [props.entry.reason, isTaskFailure ? '' : props.entry.public_code]
              .filter(Boolean)
              .join(' · ') || '—'
          }
          mono
        />
        {isTaskFailure && props.entry.public_code && (
          <DetailRow
            label={t('Error code')}
            value={props.entry.public_code}
            mono
          />
        )}
        {isTaskFailure && detail.fail_reason && (
          <DetailRow label={t('Failure reason')} value={detail.fail_reason} />
        )}
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
      {autoCheckEntries.length > 0 && (
        <DetailSection label={t('Auto check')}>
          <ChannelCheckDetails detail={Object.fromEntries(autoCheckEntries)} />
        </DetailSection>
      )}
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
          label={
            isTaskFailure
              ? t('Upstream creation request ID')
              : t('Upstream Request ID')
          }
          value={
            (isTaskFailure
              ? detail.create_upstream_request_id
              : props.entry.upstream_request_id) || t('Not recorded')
          }
          mono
        />
      </DetailSection>
      {isTaskFailure && props.entry.task_id && (
        <Button
          variant='outline'
          role='link'
          render={
            <Link
              to='/usage-logs/$section'
              params={{
                section:
                  props.entry.stage === 'midjourney' ||
                  detail.platform === 'midjourney'
                    ? 'drawing'
                    : 'task',
              }}
              search={{
                filter: props.entry.task_id,
                page: 1,
                startTime: 0,
                endTime: (props.entry.created_at + 1) * 1000,
              }}
            />
          }
        >
          {t('View task')}
        </Button>
      )}
      {showTaskEvidence && (
        <TaskEvidence
          key={props.entry.id}
          taskId={isTaskFailure ? props.entry.task_id || undefined : undefined}
          requestId={
            isTaskFailure && props.entry.task_id
              ? undefined
              : props.entry.request_id || undefined
          }
          isRoot={isRoot}
        />
      )}
      <ErrorLogHTTPExchange
        detail={props.entry.detail}
        hideWhenEmpty={isTaskFailure}
      />
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
