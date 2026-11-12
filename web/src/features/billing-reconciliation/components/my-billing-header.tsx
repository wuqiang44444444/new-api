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
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

export function MyBillingHeader(props: {
  heading: string
  backAction?: ReactNode
  customerDetails?: ReactNode
  actions: ReactNode
  versionActions: ReactNode
  reportStatus: ReactNode
  versionStatus: ReactNode
  pendingRefund: ReactNode
  versionNotice: ReactNode
  qualityNotice: ReactNode
}) {
  const { t } = useTranslation()

  return (
    <header className='space-y-2 pt-3 sm:pt-4'>
      <div className='flex flex-wrap items-center justify-between gap-x-4 gap-y-2'>
        <div className='flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1'>
          {props.backAction}
          <h2 className='text-base font-bold tracking-tight wrap-anywhere sm:text-lg'>
            {props.heading}
          </h2>
          {props.customerDetails}
        </div>
        <div className='flex min-w-0 flex-wrap items-center gap-2 sm:ms-auto'>
          {props.actions}
          {props.versionActions}
        </div>
      </div>
      <div className='text-muted-foreground flex flex-wrap items-center gap-x-3 gap-y-1.5 text-xs wrap-anywhere [&_[data-slot=badge]]:h-auto [&_[data-slot=badge]]:max-w-full [&_[data-slot=badge]]:whitespace-normal'>
        <p>
          <span>{t('Database authoritative')}</span> ·{' '}
          <span>{t('Asia/Shanghai settlement period')}</span>
        </p>
        {props.reportStatus}
        {props.versionStatus}
        {props.pendingRefund}
      </div>
      {props.versionNotice}
      {props.qualityNotice}
    </header>
  )
}
