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
import { Download01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { toIntlLocale } from '@/i18n/languages'

import type { CustomerExportJobView } from '../export-api'
import { ExportJobsDrawer } from '../export-jobs-drawer'
import { formatBillingMonth } from '../lib'
import { useBillingExportJob } from '../use-billing-export-job'
import { BillingExportSection } from './billing-export-section'

export function BillingExportAction(props: {
  submit: () => Promise<CustomerExportJobView>
  startTimestamp: number
  statement: string
  description: string
  notice?: string
  upstreamBilling?: boolean
  scope: string[]
  available: boolean
  unavailableNotice?: string
  className?: string
}) {
  const { t, i18n } = useTranslation()
  const [open, setOpen] = useState(false)
  const job = useBillingExportJob(props.submit)
  const month = formatBillingMonth(
    props.startTimestamp,
    toIntlLocale(i18n?.language)
  )

  return (
    <>
      <Button
        variant='outline'
        size='sm'
        className={props.className}
        aria-haspopup='dialog'
        onClick={() => {
          job.clearFocus()
          setOpen(true)
        }}
      >
        <HugeiconsIcon
          icon={Download01Icon}
          strokeWidth={2}
          data-icon='inline-start'
        />
        {t('Export and download')}
      </Button>
      <ExportJobsDrawer
        open={open}
        upstreamBilling={props.upstreamBilling}
        billingStatement={!props.upstreamBilling}
        focusedJobId={job.focusedJobId}
        onOpenChange={(next) => {
          setOpen(next)
          if (!next) job.clearFocus()
        }}
      >
        <BillingExportSection
          title={t('{{month}} {{statement}}', {
            month,
            statement: props.statement,
          })}
          actionLabel={job.isPending ? t('Loading...') : t('Export CSV')}
          disabled={!props.available || job.isPending}
          notice={
            props.available
              ? props.notice
              : props.unavailableNotice ||
                t('The current statement is unavailable.')
          }
          onSubmit={() => job.mutate()}
        >
          <ul className='space-y-1 text-sm wrap-anywhere'>
            {[...new Set(props.scope.filter(Boolean))].map((line) => (
              <li key={line}>{line}</li>
            ))}
          </ul>
          <p className='text-muted-foreground text-sm'>{props.description}</p>
        </BillingExportSection>
      </ExportJobsDrawer>
    </>
  )
}
