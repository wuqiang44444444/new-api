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
import { useTranslation } from 'react-i18next'

import { formatTimestampToDate } from '@/lib/format'

const counts = [
  ['scanned', 'Indexes scanned'],
  ['cleaned_indexes', 'Indexes cleaned'],
  ['deleted_files', 'Files removed'],
  ['freed_bytes', 'Bytes removed'],
  ['missing_files', 'Missing objects'],
  ['failed_items', 'Failed items'],
  ['unattributed_files', 'Unaccounted files'],
  ['deleted_events', 'Event rows removed'],
  ['deleted_indexes', 'Index rows removed'],
  ['deleted_access_logs', 'Access audit rows removed'],
  ['deleted_history', 'Evidence task history rows removed'],
  ['database_failures', 'Database cleanup failures'],
] as const

const errorLabels: Record<string, string> = {
  invalid_payload: 'Invalid cleanup scope',
  store_unavailable: 'Evidence storage unavailable',
  scan_failed: 'Failed to scan evidence indexes',
  mark_failed: 'Failed to save evidence cleanup state',
  list_events_failed: 'Failed to load evidence events',
  delete_failed: 'Failed to delete evidence objects',
  directory_cleanup_failed: 'Failed to remove evidence directories',
  database_cleanup_failed: 'Failed to remove evidence database records',
  cancelled: 'Cleanup cancelled',
}

export function EvidenceCleanupSummary(props: {
  stats: unknown
  payload?: unknown
}) {
  const { t } = useTranslation()
  const stats =
    props.stats && typeof props.stats === 'object'
      ? (props.stats as Record<string, unknown>)
      : null
  const payload =
    props.payload && typeof props.payload === 'object'
      ? (props.payload as Record<string, unknown>)
      : null
  const cutoff = stats?.target_timestamp ?? payload?.target_timestamp
  const trigger = stats?.trigger ?? payload?.trigger ?? 'scheduled'
  const category = stats?.error_category
  return (
    <div className='space-y-2 text-sm'>
      <p>
        {trigger === 'manual' ? t('Manual cleanup') : t('Scheduled cleanup')}
      </p>
      {typeof cutoff === 'number' && cutoff > 0 && (
        <p>
          {t('Cleanup cutoff')}: {formatTimestampToDate(cutoff)}
        </p>
      )}
      {stats && (
        <dl className='grid grid-cols-1 gap-x-6 gap-y-1 sm:grid-cols-2'>
          {counts.map(([key, label]) => (
            <div
              key={key}
              role='group'
              aria-label={t(label)}
              className='flex min-w-0 justify-between gap-4'
            >
              <dt className='text-muted-foreground'>{t(label)}</dt>
              <dd className='shrink-0 tabular-nums'>
                {typeof stats[key] === 'number'
                  ? String(stats[key])
                  : t('Unavailable')}
              </dd>
            </div>
          ))}
        </dl>
      )}
      {!stats && <p>{t('Waiting for cleanup statistics')}</p>}
      {typeof category === 'string' && category && (
        <p className='text-destructive'>
          {t(errorLabels[category] ?? 'Failed to clean request evidence')}
        </p>
      )}
      {typeof stats?.unattributed_files === 'number' &&
        stats.unattributed_files > 0 && (
          <p className='text-destructive'>
            {t('Unaccounted files are retained for administrator inspection.')}
          </p>
        )}
    </div>
  )
}
