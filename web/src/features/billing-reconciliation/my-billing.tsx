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

import { Main } from '@/components/layout'
import { Input } from '@/components/ui/input'

import { CustomerStatementView } from './components/customer-statement'
import { currentShanghaiMonth, resolveShanghaiMonth } from './lib'

type MyBillingProps = {
  month?: string
  modelKeyId?: number
  onModelKeyChange: (id?: number) => void
  onMonthChange: (month: string) => void
}

export function MyBilling(props: MyBillingProps) {
  const { t } = useTranslation()
  const month = props.month ?? currentShanghaiMonth()
  const period = resolveShanghaiMonth(month)

  return (
    <Main className='px-1 sm:px-2'>
      <CustomerStatementView
        heading={t('My billing')}
        modelKeyId={props.modelKeyId}
        onModelKeyChange={props.onModelKeyChange}
        toolbar={
          <label className='flex max-w-full flex-wrap items-center gap-x-2 gap-y-1'>
            <span className='text-muted-foreground text-xs font-medium'>
              {t('Billing month')}
            </span>
            <Input
              className='w-36'
              type='month'
              value={month}
              onChange={(event) =>
                props.onMonthChange(
                  event.target.value || currentShanghaiMonth()
                )
              }
            />
          </label>
        }
        isAdmin={false}
        period={period}
        dimension='api_key'
        onDimensionChange={() => undefined}
      />
    </Main>
  )
}
