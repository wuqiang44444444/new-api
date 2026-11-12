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
  billingEvidenceGroups,
  upstreamEvidenceCoverageLabel,
} from '../upstream-statement-utils'
import { UpstreamEvidenceCoverage } from './upstream-evidence-coverage'
import { UpstreamEvidenceLink } from './upstream-evidence-link'

// Amount availability and usage evidence are independent. Test coverage is a
// scope note. Missing test amounts invalidate complete totals; usage-only gaps
// do not invalidate independently known money.
export function UpstreamDataStatus(props: {
  scope?: 'report'
  quality?: BillingDataQuality
  originalAmount?: number | null
  usageOnly?: boolean
  onViewEvidence?: (entry: BillingEvidenceEntry) => void
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const trigger = useRef<HTMLButtonElement>(null)
  const openingEvidence = useRef(false)
  const evidenceGroups = billingEvidenceGroups(props.quality, t)
  const accountingNotes = billingAccountingEntries(props.quality, t)
  const allEntries = [
    ...evidenceGroups.flatMap((group) => group.entries),
    ...accountingNotes,
  ]
  const isReport = props.scope === 'report'
  const coverage = props.quality?.evidence_coverage
  const status = billingDataQualityLabel(props.quality, t)
  let label = status
  if (isReport) {
    label = coverage
      ? upstreamEvidenceCoverageLabel(coverage, t)
      : t('Data status')
  }
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
      {!isReport && (
        <span className='text-xs whitespace-nowrap'>{amountLabel}</span>
      )}
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
              size={isReport ? 'sm' : 'xs'}
              className={
                isReport
                  ? 'h-auto min-h-7 text-start whitespace-normal'
                  : undefined
              }
              aria-label={
                isReport
                  ? undefined
                  : `${label}: ${allEntries.map((entry) => entry.text).join(' ')}`
              }
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
          {isReport ? (
            <p className='text-muted-foreground text-xs'>
              {t(
                'Upstream names and monthly discounts are entered manually in channel management. Calculated amounts are references, not supplier bills. Missing amounts remain unknown.'
              )}
            </p>
          ) : (
            <p className='text-xs'>{amountLabel}</p>
          )}
          <UpstreamEvidenceCoverage
            quality={props.quality}
            onViewEvidence={viewEvidence}
          />
          {isReport && !props.onViewEvidence && (
            <p className='text-muted-foreground text-xs'>
              {t('Open a matching channel to view records for this search.')}
            </p>
          )}
          {evidenceGroups.length > 0 && (
            <>
              <p className='text-xs font-medium'>
                {t('Evidence gap categories')}
              </p>
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Except for the mutually exclusive channel-test breakdown, categories may overlap on the same record; do not add these counts together.'
                )}
              </p>
              {evidenceGroups.map((group) => (
                <div key={group.key} className='space-y-1 text-xs'>
                  {group.title ? (
                    <p className='font-medium'>{group.title}</p>
                  ) : null}
                  <ul className='list-disc space-y-1 pl-4'>
                    {group.entries.map((entry) => (
                      <li key={entry.filter || entry.text}>
                        {entry.text}
                        <UpstreamEvidenceLink
                          entry={entry}
                          onViewEvidence={viewEvidence}
                        />
                      </li>
                    ))}
                  </ul>
                </div>
              ))}
            </>
          )}
          {accountingNotes.length > 0 && (
            <div className='text-muted-foreground space-y-1 text-xs'>
              <p className='font-medium'>{t('Confirmed accounting results')}</p>
              {accountingNotes.map((note) => (
                <p key={note.filter || note.text}>
                  {note.text}
                  <UpstreamEvidenceLink
                    entry={note}
                    onViewEvidence={viewEvidence}
                  />
                </p>
              ))}
            </div>
          )}
        </PopoverContent>
      </Popover>
    </div>
  )
}
