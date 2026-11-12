import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { LOG_TYPE_ENUM } from '@/features/usage-logs/constants'

import { formatCustomerStatementQuota } from '../lib'
import type { SourceDecisionRecord } from '../source-review-api'
import { BillingPagination } from './billing-pagination'
import { useSourceDecisionLabels } from './source-decision-labels'

export function SourceDecisionHistory(props: {
  history: SourceDecisionRecord[]
}) {
  const { t } = useTranslation()
  const labels = useSourceDecisionLabels()
  const [page, setPage] = useState(0)
  const [pageSize, setPageSize] = useState(10)
  const pages = Math.max(1, Math.ceil(props.history.length / pageSize))
  const current = Math.min(page, pages - 1)
  const decisions: Record<string, string> = labels.decisions
  const reasons: Record<string, string> = labels.reasons
  const newest = [...props.history].reverse()
  const visible = newest.slice(current * pageSize, (current + 1) * pageSize)
  return (
    <Collapsible className='rounded-md border p-3'>
      <CollapsibleTrigger render={<Button variant='outline' />}>
        {t('Handling history ({{count}})', { count: props.history.length })}
      </CollapsibleTrigger>
      <CollapsibleContent>
        {!props.history.length && (
          <p className='mt-3 text-sm'>
            {t('No handling decisions have been recorded.')}
          </p>
        )}
        <ol className='mt-3 space-y-3'>
          {visible.map((record) => (
            <li key={record.id} className='space-y-1 border-t pt-3 text-sm'>
              <p className='font-medium'>
                {decisions[record.decision] || t('Historical review note')}
              </p>
              <p>
                {t('Administrator #{{actor}} · {{time}}', {
                  actor: record.actor_id,
                  time: new Date(record.created_at * 1000).toLocaleString(),
                })}
              </p>
              <p>
                {record.model} · {record.issue_id}
              </p>
              {(record.version === 2 || record.version === 3) && (
                <>
                  <p>
                    {record.kind === 'task_amount'
                      ? t('Linked records net amount')
                      : t('Recorded amount at the time')}
                    :{' '}
                    {formatCustomerStatementQuota(
                      Number(record.recorded_quota)
                    )}{' '}
                    ·{' '}
                    {record.log_type === LOG_TYPE_ENUM.REFUND
                      ? t('Refund')
                      : t('Billing record')}
                  </p>
                  {record.status === 'small_difference_closed' &&
                    record.target_quota != null && (
                      <p>
                        {t(
                          'Task settlement {{amount}} · Difference {{difference}}',
                          {
                            amount: formatCustomerStatementQuota(
                              Number(record.target_quota)
                            ),
                            difference: formatCustomerStatementQuota(
                              Math.abs(
                                Number(record.target_quota) -
                                  Number(record.recorded_quota)
                              )
                            ),
                          }
                        )}
                      </p>
                    )}
                  <p>{reasons[record.reason] || record.reason}</p>
                  <p>
                    {record.status === 'record_kept' &&
                      t('Decision recorded; existing record retained.')}
                    {record.status === 'pending_review' &&
                      t(
                        'Request recorded; processing is pending. This is not a completed adjustment or refund.'
                      )}
                    {record.status === 'request_withdrawn' &&
                      t(
                        'Request withdrawn. No accounting adjustment was executed.'
                      )}
                    {record.status === 'request_rejected' &&
                      t(
                        'Request rejected. No accounting adjustment was executed.'
                      )}
                  </p>
                  <p>
                    {t(
                      'Recorded statement adjustment: {{statement}}; customer balance change: {{balance}}.',
                      {
                        statement: formatCustomerStatementQuota(
                          Number(record.statement_delta)
                        ),
                        balance: formatCustomerStatementQuota(
                          Number(record.balance_delta)
                        ),
                      }
                    )}
                  </p>
                </>
              )}
              {record.note && (
                <p className='break-words whitespace-pre-wrap'>{record.note}</p>
              )}
              {record.previous_id > 0 && (
                <p>
                  {t(
                    'Replaces decision #{{id}}; the original remains in history.',
                    { id: record.previous_id }
                  )}
                </p>
              )}
              {record.superseded && (
                <p className='text-muted-foreground'>
                  {t(
                    'Replaced by a later decision. This is no longer the current instruction.'
                  )}
                </p>
              )}
              {!record.current_source && (
                <p className='text-muted-foreground'>
                  {t(
                    'Historical context only. This decision does not verify the current records.'
                  )}
                </p>
              )}
            </li>
          ))}
        </ol>
        {newest.length > 0 && (
          <BillingPagination
            page={current + 1}
            pageSize={pageSize}
            total={newest.length}
            label={t('Handling history ({{count}})', {
              count: props.history.length,
            })}
            onChange={(nextPage, nextSize) => {
              setPage(nextPage - 1)
              setPageSize(nextSize)
            }}
          />
        )}
      </CollapsibleContent>
    </Collapsible>
  )
}
