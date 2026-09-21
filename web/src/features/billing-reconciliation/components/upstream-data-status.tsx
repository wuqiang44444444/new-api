/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
  PopoverTitle,
} from '@/components/ui/popover'

import type { BillingDataQuality } from '../types'
import {
  type BillingEvidenceEntry,
  billingAccountingEntries,
  billingDataQualityLabel,
  billingDataQualityEntries,
} from '../upstream-statement-utils'
import { UpstreamEvidenceCoverage } from './upstream-evidence-coverage'
import { UpstreamEvidenceLink } from './upstream-evidence-link'

// Amount availability and usage evidence are independent. Test coverage is a
// scope note. Missing test amounts invalidate complete totals; usage-only gaps
// do not invalidate independently known money.
export function UpstreamDataStatus(props: {
  quality?: BillingDataQuality
  originalAmount?: number | null
  usageOnly?: boolean
  onViewEvidence?: (entry: BillingEvidenceEntry) => void
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const trigger = useRef<HTMLButtonElement>(null)
  const openingEvidence = useRef(false)
  const reasons = billingDataQualityEntries(props.quality, t, false)
  const label = billingDataQualityLabel(props.quality, t)
  let amountLabel = t('Original amount complete')
  if (props.usageOnly) {
    amountLabel = t('Channel tests only; amounts pending')
  } else if (props.originalAmount == null) {
    amountLabel = t('Original amount incomplete')
  }
  const viewEvidence = props.onViewEvidence
    ? (entry: BillingEvidenceEntry) => {
        openingEvidence.current = true
        trigger.current?.focus()
        setOpen(false)
        props.onViewEvidence?.(entry)
      }
    : undefined
  return (
    <div className='flex flex-col items-start gap-1'>
      <span className='text-xs whitespace-nowrap'>{amountLabel}</span>
      <Popover
        open={open}
        onOpenChange={(next) => {
          if (next) openingEvidence.current = false
          setOpen(next)
        }}
      >
        <PopoverTrigger
          render={
            <Button
              ref={trigger}
              variant='outline'
              size='xs'
              aria-label={`${label}: ${reasons.map((entry) => entry.text).join(' ')}`}
            />
          }
        >
          {label}
        </PopoverTrigger>
        <PopoverContent
          finalFocus={() => (openingEvidence.current ? false : trigger.current)}
          align='end'
          className='max-h-96 w-80 max-w-[calc(100vw-2rem)] overflow-y-auto whitespace-normal'
        >
          <PopoverTitle>{t('Data status')}</PopoverTitle>
          <p className='text-xs'>{amountLabel}</p>
          <UpstreamEvidenceCoverage
            quality={props.quality}
            onViewEvidence={viewEvidence}
          />
          {reasons.length > 0 && (
            <>
              <p className='text-xs font-medium'>
                {t('Evidence gap categories')}
              </p>
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Categories can overlap on the same record; do not add these counts together.'
                )}
              </p>
              <ul className='list-disc space-y-1 pl-4 text-xs'>
                {reasons.map((reason) => (
                  <li key={reason.filter}>
                    {reason.text}
                    <UpstreamEvidenceLink
                      entry={reason}
                      onViewEvidence={viewEvidence}
                    />
                  </li>
                ))}
              </ul>
            </>
          )}
          {billingAccountingEntries(props.quality, t).map((note) => (
            <p key={note.filter} className='text-muted-foreground text-xs'>
              {note.text}
              <UpstreamEvidenceLink
                entry={note}
                onViewEvidence={viewEvidence}
              />
            </p>
          ))}
        </PopoverContent>
      </Popover>
    </div>
  )
}
