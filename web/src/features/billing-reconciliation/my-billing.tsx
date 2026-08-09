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

import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'

import { CustomerStatementView } from './components/customer-statement'
import { currentShanghaiMonth, resolveShanghaiMonth } from './lib'

type MyBillingProps = {
  month?: string
  onMonthChange: (month: string) => void
}

export function MyBilling(props: MyBillingProps) {
  const { t } = useTranslation()
  const month = props.month ?? currentShanghaiMonth()
  const period = resolveShanghaiMonth(month)

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        <span className='inline-flex min-w-0 items-center gap-2'>
          <span className='truncate'>{t('My billing')}</span>
          <Badge variant='outline'>{t('Database authoritative')}</Badge>
        </span>
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='space-y-3'>
          <div className='flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between'>
            <div>
              <p className='text-muted-foreground text-sm'>
                {t(
                  'Review settled totals by API key and expand models when needed.'
                )}
              </p>
              <p className='text-muted-foreground mt-1 text-xs'>
                {t(
                  'Request-level records are available only through detail links.'
                )}
              </p>
            </div>
            <label className='w-full space-y-1.5 sm:w-44'>
              <span className='text-muted-foreground text-xs font-medium'>
                {t('Billing month')}
              </span>
              <Input
                type='month'
                value={month}
                onChange={(event) =>
                  props.onMonthChange(
                    event.target.value || currentShanghaiMonth()
                  )
                }
              />
            </label>
          </div>

          <CustomerStatementView
            isAdmin={false}
            period={period}
            dimension='api_key'
            onDimensionChange={() => undefined}
          />
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
