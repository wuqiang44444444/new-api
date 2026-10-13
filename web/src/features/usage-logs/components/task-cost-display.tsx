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
import { formatLogQuota } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { TaskCostSummary } from '../lib/task-cost'

export function TaskCostDisplay(props: {
  summary: TaskCostSummary
  subscription: boolean
}) {
  const { t } = useTranslation()
  const summary = props.summary
  const rows = [
    {
      label: t(summary.beforeLabel),
      value: summary.before,
      sign: '',
    },
    {
      label: t(summary.changeLabel),
      value: summary.change,
      sign: summary.refund ? '−' : '+',
    },
    {
      label: t(summary.afterLabel),
      value: summary.after,
      sign: '',
    },
  ]

  return (
    <div className='min-w-0 space-y-1'>
      {props.subscription ? (
        <>
          <StatusBadge
            label={t('Subscription')}
            variant='success'
            size='sm'
            copyable={false}
          />
          <p className='text-muted-foreground text-xs'>
            {t('Amounts are converted subscription quota.')}
          </p>
        </>
      ) : null}
      <ul
        aria-label={t('Task cost breakdown')}
        className='min-w-0 space-y-1 text-xs tabular-nums'
      >
        {rows.map((row, index) => (
          <li
            key={row.label}
            className={cn(
              'flex min-w-0 flex-wrap items-baseline justify-between gap-x-3 gap-y-0.5',
              index === 0 && 'text-muted-foreground',
              index === 1 && summary.refund && 'text-success',
              index === 2 && 'font-semibold'
            )}
          >
            <span>{row.label}</span>
            <span className='break-all'>
              {row.value === null
                ? t('Not recorded')
                : `${row.sign}${formatLogQuota(row.value)}`}
            </span>
          </li>
        ))}
      </ul>
    </div>
  )
}
