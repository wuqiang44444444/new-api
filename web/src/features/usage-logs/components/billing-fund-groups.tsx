import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatCustomerStatementQuota } from '@/features/billing-reconciliation/lib'

import type { BillingFundGroup } from '../types'
import { useUsageLogsContext } from './usage-logs-provider'

export function BillingFundGroups(props: {
  rows: BillingFundGroup[]
  quota?: number
  previous: boolean
  next: boolean
  loading: boolean
  onPage: (offset: number) => void
  onSelect: (group: string) => void
}) {
  const { t } = useTranslation()
  const { sensitiveVisible } = useUsageLogsContext()
  const money = (quota?: number) =>
    sensitiveVisible ? formatCustomerStatementQuota(quota) : '••••'
  return (
    <div className='flex min-h-0 flex-1 flex-col gap-3'>
      <p className='font-medium'>
        {t('Net amount')} {money(props.quota)}
      </p>
      <div className='min-h-0 flex-1 overflow-auto'>
        <BillingFundGroupTable
          rows={props.rows}
          loading={props.loading}
          formatAmount={money}
          onSelect={props.onSelect}
        />
        {props.rows.length === 0 && (
          <p className='text-muted-foreground p-4'>{t('No Logs Found')}</p>
        )}
      </div>
      <div className='flex justify-end gap-2'>
        <Button
          variant='outline'
          disabled={props.loading || !props.previous}
          onClick={() => props.onPage(-1)}
        >
          {t('Previous page')}
        </Button>
        <Button
          variant='outline'
          disabled={props.loading || !props.next}
          onClick={() => props.onPage(1)}
        >
          {t('Next page')}
        </Button>
      </div>
    </div>
  )
}

export function BillingFundGroupTable(props: {
  rows: BillingFundGroup[]
  loading: boolean
  formatAmount: (quota: number) => string
  onSelect: (group: string) => void
}) {
  const { t } = useTranslation()
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('Day')}</TableHead>
          <TableHead>{t('Model')}</TableHead>
          <TableHead>{t('Net amount')}</TableHead>
          <TableHead>
            <span className='sr-only'>{t('View details')}</span>
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {props.rows.map((row) => (
          <TableRow key={row.key}>
            <TableCell>{row.date}</TableCell>
            <TableCell>{row.model_name}</TableCell>
            <TableCell className='tabular-nums'>
              {props.formatAmount(row.net_quota)}
            </TableCell>
            <TableCell>
              <Button
                variant='link'
                size='sm'
                disabled={props.loading}
                onClick={() => props.onSelect(row.key)}
              >
                {t('View details')} ({row.event_count})
              </Button>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
