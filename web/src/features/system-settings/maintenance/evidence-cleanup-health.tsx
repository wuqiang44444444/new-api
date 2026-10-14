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

const alertLabels: Record<string, string> = {
  consecutive_failures: 'Evidence cleanup failed in two consecutive runs.',
  cleanup_stalled: 'Evidence cleanup has not succeeded for two hours.',
  cleanup_backlog: 'Request evidence cleanup is backlogged.',
  disk_space_low: 'Evidence storage disk space is low.',
  inodes_low: 'Evidence storage inodes are nearly exhausted.',
}

export function EvidenceCleanupHealth(props: { health: unknown }) {
  const { t } = useTranslation()
  if (!props.health || typeof props.health !== 'object') return null
  const health = props.health as Record<string, unknown>
  const alerts = Array.isArray(health.alerts)
    ? health.alerts.filter(
        (value): value is string => typeof value === 'string'
      )
    : []
  return (
    <div className='space-y-2 text-sm'>
      <p className='font-medium'>{t('Evidence cleanup monitoring')}</p>
      <dl className='grid grid-cols-1 gap-2 sm:grid-cols-2'>
        <div role='group' aria-label={t('Pending indexes')}>
          <dt>{t('Pending indexes')}</dt>
          <dd>
            {typeof health.pending_indexes === 'number'
              ? health.pending_indexes
              : t('Unavailable')}
          </dd>
        </div>
        <div role='group' aria-label={t('Pending database indexes')}>
          <dt>{t('Pending database indexes')}</dt>
          <dd>
            {typeof health.pending_database_indexes === 'number'
              ? health.pending_database_indexes
              : t('Unavailable')}
          </dd>
        </div>
        <div>
          <dt>{t('Oldest pending database evidence')}</dt>
          <dd>
            {typeof health.oldest_database_pending_at === 'number' && health.oldest_database_pending_at > 0
              ? formatTimestampToDate(health.oldest_database_pending_at)
              : t('Unavailable')}
          </dd>
        </div>
        <div role='group' aria-label={t('Free disk bytes')}>
          <dt>{t('Free disk bytes')}</dt>
          <dd>
            {typeof health.free_bytes === 'number'
              ? health.free_bytes
              : t('Unavailable')}
          </dd>
        </div>
        <div role='group' aria-label={t('Free inodes')}>
          <dt>{t('Free inodes')}</dt>
          <dd>
            {typeof health.free_inodes === 'number'
              ? health.free_inodes
              : t('Unavailable')}
          </dd>
        </div>
        <div>
          <dt>{t('Last successful cleanup')}</dt>
          <dd>
            {typeof health.last_succeeded_at === 'number' &&
            health.last_succeeded_at > 0
              ? formatTimestampToDate(health.last_succeeded_at)
              : t('Unavailable')}
          </dd>
        </div>
        <div>
          <dt>{t('Checked at')}</dt>
          <dd>
            {typeof health.checked_at === 'number'
              ? formatTimestampToDate(health.checked_at)
              : t('Unavailable')}
          </dd>
        </div>
      </dl>
      {alerts.length > 0 && (
        <ul role='alert' className='text-destructive list-inside list-disc'>
          {alerts.map((alert) => (
            <li key={alert}>
              {t(alertLabels[alert] ?? 'Evidence cleanup health check failed.')}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
