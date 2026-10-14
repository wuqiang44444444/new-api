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
import { EvidenceCleanupHealth } from '@/features/system-settings/maintenance/evidence-cleanup-health'
import { EvidenceCleanupSummary } from '@/features/system-settings/maintenance/evidence-cleanup-summary'
import type { SystemTask } from '@/features/system-settings/types'

export function EvidenceCleanupResultsDialog(props: { task: SystemTask }) {
  const { t } = useTranslation()
  return (
    <Dialog
      title={t('Evidence cleanup results')}
      trigger={
        <Button variant='ghost' size='sm'>
          {t('Cleanup results')}
        </Button>
      }
    >
      {props.task.type === 'task_request_evidence_monitor' ? (
        <EvidenceCleanupHealth
          health={props.task.result ?? { alerts: ['health_check_failed'] }}
        />
      ) : (
        <EvidenceCleanupSummary
          stats={props.task.result ?? props.task.state}
          payload={props.task.payload}
        />
      )}
    </Dialog>
  )
}
