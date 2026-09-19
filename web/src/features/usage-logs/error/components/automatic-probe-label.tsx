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

import { StatusBadge } from '@/components/status-badge'

import type { ErrorLogItem } from '../api'
import { mediaAutoProbeDetail } from './auto-probe-detail'
import { errorEventTypeLabel } from './error-event-type'

export function AutomaticProbeLabel(props: { entry: ErrorLogItem }) {
  const { t } = useTranslation()
  const detail = mediaAutoProbeDetail(props.entry)
  if (!detail) {
    return (
      <span className='text-muted-foreground'>
        {errorEventTypeLabel(props.entry.event_type, t)}
      </span>
    )
  }
  let label = t('Automatic probe')
  if (!detail.check_scope) {
    label = t('Automatic probe (historical)')
  }
  if (detail.check_scope === 'readonly_probe') {
    label = t('Automatic connection probe')
  }
  if (
    detail.check_scope === 'config_only' ||
    detail.check_scope === 'target_unresolved'
  ) {
    label = t('Automatic configuration check')
  }
  if (detail.check_scope === 'generation_probe') {
    label = t('Automatic generation probe (historical)')
  }
  return (
    <div className='space-y-1'>
      <StatusBadge label={label} variant='info' copyable={false} />
      {detail.probe_media === 'image' && (
        <span className='text-muted-foreground block text-xs'>
          {t('Image probe')}
        </span>
      )}
      {detail.probe_media === 'video' && (
        <span className='text-muted-foreground block text-xs'>
          {t('Video probe')}
        </span>
      )}
      <span className='text-muted-foreground block text-xs'>
        {t('Not a customer request')}
      </span>
    </div>
  )
}

export function AutomaticProbeStatus(props: {
  entry: ErrorLogItem
  detail: Record<string, string>
}) {
  const { t } = useTranslation()
  const recorded = Number(props.detail.upstream_status)
  const status =
    Number.isInteger(recorded) && recorded >= 100 && recorded <= 599
      ? recorded
      : 0
  let label = t('Connection not verified')
  let variant: 'info' | 'warning' | 'danger' = 'info'
  if (status > 0) {
    label = t('Response received')
    if (status === 401 || status === 403) {
      label = t('Response received; authentication or permission issue')
      variant = 'warning'
    } else if (status === 429) {
      label = t('Response received; rate limited')
      variant = 'warning'
    } else if (status >= 500) {
      label = t('Service or gateway issue')
      variant = 'danger'
    } else if (
      status === 404 ||
      status === 405 ||
      (status === 400 &&
        props.entry.public_code === 'model_not_supported' &&
        (!props.detail.check_scope ||
          props.detail.check_scope === 'generation_probe'))
    ) {
      label = t('Response received; probe not applicable')
    } else if (status >= 400) {
      label = t('Response received; probe request rejected')
      variant = 'warning'
    }
  } else if (props.detail.upstream_request === 'not_sent') {
    label = t('Request not sent')
    if (props.detail.config_check === 'failed') variant = 'warning'
  } else if (props.detail.upstream_request === 'response_received') {
    label = t('Response received')
    if (props.detail.check_result === 'failed') {
      label = t('Response received; probe did not pass')
      variant = 'warning'
    }
  } else if (
    props.detail.connection_result === 'connection_error' ||
    props.entry.reason === 'test_upstream_unreachable'
  ) {
    label = t('Connection error')
    variant = 'danger'
  }
  return (
    <div className='space-y-1'>
      <StatusBadge label={label} variant={variant} copyable={false} />
      {status > 0 && (
        <span className='text-muted-foreground block text-xs'>
          {t('Upstream HTTP {{status}}', { status })}
        </span>
      )}
    </div>
  )
}
