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
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import { CustomerStatementView } from './components/customer-statement'
import { CustomerStatementsListView } from './components/customer-statements-list'
import { UpstreamStatementView } from './components/upstream-statement'
import { currentShanghaiMonth, resolveShanghaiMonth } from './lib'
import type {
  AdminBillingSearch,
  BillingDimension,
  BillingSection,
} from './types'

type AdminBillingReconciliationProps = {
  search: AdminBillingSearch
  onSearchChange: (
    patch: Partial<AdminBillingSearch>,
    replace?: boolean
  ) => void
}

export function AdminBillingReconciliation(
  props: AdminBillingReconciliationProps
) {
  const { t } = useTranslation()
  const section: BillingSection = props.search.section ?? 'customer'
  const month = props.search.month ?? currentShanghaiMonth()
  const dimension: BillingDimension = props.search.dimension ?? 'api_key'
  const period = resolveShanghaiMonth(month)
  const isUpstream = section === 'upstream'
  const pageTitle = isUpstream
    ? t('Upstream reconciliation')
    : t('Billing reconciliation')
  const pageDescription = isUpstream
    ? t('Summarize platform-recorded Token usage or billable calls by channel.')
    : t(
        'Customer charges and platform-recorded upstream usage use separate views.'
      )

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        <span className='inline-flex min-w-0 items-center gap-2'>
          <span className='truncate'>{pageTitle}</span>
          <Badge variant='outline'>{t('Database authoritative')}</Badge>
        </span>
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='space-y-3'>
          <div className='flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between'>
            <div>
              <p className='text-muted-foreground text-sm'>{pageDescription}</p>
              <p className='text-muted-foreground mt-1 text-xs'>
                {t(
                  'Request-level records are available only through detail links.'
                )}
              </p>
            </div>
            {!isUpstream ? (
              <label className='w-full space-y-1.5 sm:w-44'>
                <span className='text-muted-foreground text-xs font-medium'>
                  {t('Billing month')}
                </span>
                <Input
                  type='month'
                  value={month}
                  onChange={(event) =>
                    props.onSearchChange({
                      month: event.target.value || currentShanghaiMonth(),
                    })
                  }
                />
              </label>
            ) : null}
          </div>

          <Tabs
            value={section}
            onValueChange={(value) =>
              value != null &&
              props.onSearchChange({ section: value as BillingSection })
            }
          >
            <TabsList
              variant='line'
              aria-label={t('Billing reconciliation sections')}
            >
              <TabsTrigger value='customer'>
                {t('Customer billing')}
              </TabsTrigger>
              <TabsTrigger value='upstream'>
                {t('Upstream reconciliation')}
              </TabsTrigger>
            </TabsList>
            <TabsContent value='customer'>
              {props.search.userId ? (
                <CustomerStatementView
                  isAdmin
                  period={period}
                  userId={props.search.userId}
                  dimension={dimension}
                  onBack={() => props.onSearchChange({ userId: undefined })}
                  onDimensionChange={(nextDimension) =>
                    props.onSearchChange({ dimension: nextDimension })
                  }
                />
              ) : (
                <CustomerStatementsListView
                  period={period}
                  search={props.search}
                  onSearchChange={props.onSearchChange}
                  onSelectUser={(userId) => props.onSearchChange({ userId })}
                />
              )}
            </TabsContent>
            <TabsContent value='upstream'>
              <UpstreamStatementView
                month={month}
                period={period}
                onMonthChange={(nextMonth) =>
                  props.onSearchChange({
                    month: nextMonth || currentShanghaiMonth(),
                  })
                }
              />
            </TabsContent>
          </Tabs>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
