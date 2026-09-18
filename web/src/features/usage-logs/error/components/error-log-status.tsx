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

export function ErrorLogStatus(props: {
  entry: ErrorLogItem
  compact?: boolean
}) {
  const { t } = useTranslation()
  let status = props.entry.status
  let label = t('No client HTTP status')
  if (status > 0) {
    label = props.compact ? String(status) : `HTTP ${status}`
  }
  if (props.entry.event_type === 'channel_test') {
    status = 0
    try {
      const detail = JSON.parse(props.entry.detail || '{}')
      const recorded = Number(detail?.upstream_status)
      if (Number.isInteger(recorded) && recorded >= 100 && recorded <= 599) {
        status = recorded
      }
    } catch {
      // Missing diagnostic metadata must not be inferred from the error code.
    }
    label =
      status > 0
        ? t('Upstream HTTP {{status}}', { status })
        : t('Upstream HTTP status not recorded')
  }
  return (
    <StatusBadge
      label={label}
      variant={status >= 500 ? 'danger' : 'warning'}
      copyable={false}
    />
  )
}
