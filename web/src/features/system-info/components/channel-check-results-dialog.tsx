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

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { ChannelCheckDetails } from '@/features/channels/components/channel-check-details'
import {
  DetailRow,
  DetailSection,
} from '@/features/usage-logs/components/dialogs/log-detail-layout'
import { formatTimestampToDate } from '@/lib/format'

type ChannelCheckResult = {
  channel_id: number
  model: string
  checked_at: number
  detail: Record<string, string>
}

// Parse persisted task results at the API boundary. Old/manual task results
// have no checks and keep their existing presentation.
export function ChannelCheckResultsDialog(props: { checks: unknown }) {
  const { t } = useTranslation()
  if (!Array.isArray(props.checks)) return null
  const checks = props.checks.filter(
    (check): check is ChannelCheckResult =>
      check &&
      typeof check === 'object' &&
      typeof check.channel_id === 'number' &&
      typeof check.model === 'string' &&
      typeof check.checked_at === 'number' &&
      check.detail &&
      typeof check.detail === 'object' &&
      !Array.isArray(check.detail) &&
      Object.values(check.detail).every((value) => typeof value === 'string')
  )
  if (!checks.length) return null
  return (
    <Dialog
      title={t('Automatic channel check results')}
      description={t(
        'Historical results for this run. Configuration and read-only checks do not verify generation availability.'
      )}
      trigger={
        <Button variant='ghost' size='sm'>
          {t('Check results')}
        </Button>
      }
      bodyClassName='space-y-3'
    >
      {checks.map((check) => (
        <DetailSection
          key={check.channel_id}
          label={`#${check.channel_id} · ${check.model}`}
        >
          <DetailRow
            label={t('Checked at')}
            value={formatTimestampToDate(check.checked_at)}
          />
          <ChannelCheckDetails detail={check.detail} />
        </DetailSection>
      ))}
    </Dialog>
  )
}
