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

import { Main, SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import { CustomerStatementView } from './components/customer-statement'
import { CustomerStatementsListView } from './components/customer-statements-list'
import { UpstreamReconciliationView } from './components/upstream-reconciliation'
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

// Let the whole tab scroll when the controls and totals may crowd out the table.
const billingTabPanelClassName =
  'flex min-h-0 flex-col overflow-y-auto max-sm:block [@media(max-height:42rem)]:block'

export function AdminBillingReconciliation(
  props: AdminBillingReconciliationProps
) {
  const { t } = useTranslation()
  // 旧地址中的 upstream_url 已并入统一的上游对账视图；缺省仍是客户账单。
  const rawSection: string | undefined = props.search.section
  const section: BillingSection =
    rawSection === 'upstream' || rawSection === 'upstream_url'
      ? 'upstream'
      : 'customer'
  const month = props.search.month ?? currentShanghaiMonth()
  const dimension: BillingDimension = props.search.dimension ?? 'api_key'
  const period = resolveShanghaiMonth(month)
  const isUpstream = section === 'upstream'
  const pageTitle = isUpstream
    ? t('Upstream reconciliation')
    : t('Billing reconciliation')
  const monthPicker = (
    <label className='flex flex-wrap items-center gap-x-2 gap-y-1'>
      <span className='text-muted-foreground text-xs font-medium'>
        {t('Billing month')}
      </span>
      <Input
        className='w-36'
        type='month'
        value={month}
        onChange={(event) =>
          props.onSearchChange({
            month: event.target.value || currentShanghaiMonth(),
            customerPage: 1,
            reportVersion: undefined,
          })
        }
      />
    </label>
  )

  if (!isUpstream && props.search.userId) {
    return (
      <Main className='px-1 sm:px-2'>
        <CustomerStatementView
          isAdmin
          period={period}
          userId={props.search.userId}
          reportVersion={props.search.reportVersion}
          dimension={dimension}
          toolbar={monthPicker}
          onBack={() =>
            props.onSearchChange({
              userId: undefined,
              reportVersion: undefined,
            })
          }
          onDimensionChange={(nextDimension) =>
            props.onSearchChange({ dimension: nextDimension })
          }
        />
      </Main>
    )
  }

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>
        {isUpstream ? (
          <span className='inline-flex min-w-0 items-center gap-2'>
            <span className='truncate'>{pageTitle}</span>
            <Badge variant='outline'>{t('Database authoritative')}</Badge>
          </span>
        ) : (
          pageTitle
        )}
      </SectionPageLayout.Title>
      {!isUpstream && (
        <SectionPageLayout.Actions>{monthPicker}</SectionPageLayout.Actions>
      )}
      <SectionPageLayout.Content>
        <div className='flex h-full min-h-0 flex-col gap-3'>
          <Tabs
            className='min-h-0 flex-1'
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
            <TabsContent className={billingTabPanelClassName} value='customer'>
              <CustomerStatementsListView
                period={period}
                search={props.search}
                onSearchChange={props.onSearchChange}
                onSelectUser={(userId, reportVersion) =>
                  props.onSearchChange({ userId, reportVersion })
                }
              />
            </TabsContent>
            <TabsContent className={billingTabPanelClassName} value='upstream'>
              <UpstreamReconciliationView
                month={month}
                period={period}
                onMonthChange={(nextMonth) =>
                  props.onSearchChange({
                    month: nextMonth || currentShanghaiMonth(),
                    reportVersion: undefined,
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
