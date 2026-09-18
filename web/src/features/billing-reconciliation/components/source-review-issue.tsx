import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleTrigger,
  CollapsibleContent,
} from '@/components/ui/collapsible'
import { LOG_TYPE_ENUM } from '@/features/usage-logs/constants'

import { formatCustomerStatementQuota } from '../lib'
import type { SourceIssue, SourceReviewScope } from '../source-review-api'
import {
  SourceDecisionForm,
  type SourceDecisionDraft,
} from './source-decision-form'

export function SourceReviewIssue(props: {
  issue: SourceIssue
  scope: SourceReviewScope
  fingerprint: string
  canWrite: boolean
  disabled?: boolean
  disappeared?: boolean
  pendingRequest?: boolean
  draft?: SourceDecisionDraft
  onDraftChange: (draft: SourceDecisionDraft | undefined) => void
  onSaved: () => void
}) {
  const { t } = useTranslation()
  const reasons: Record<string, string> = {
    invalid_quota: t('The recorded amount is invalid.'),
    unknown_billing_mode: t(
      'Billing method was not recorded or cannot be proven.'
    ),
    input_usage_missing: t('Input usage cannot be fully recovered.'),
    cache_usage_missing: t('Cache creation usage cannot be fully recovered.'),
    metadata_unreadable: t(
      'Historical billing metadata is incomplete or unreadable.'
    ),
    auxiliary_charge: t(
      'Additional charges require separate pricing evidence.'
    ),
    task_log_net_mismatch: t(
      'The final task amount differs from explicitly related consumption and refunds.'
    ),
    no_explicit_task_logs: t(
      'No explicit matching consumption records were found.'
    ),
    identity_conflict: t(
      'Related records have conflicting ownership or billing identities.'
    ),
    settlement_target_conflict: t('Task settlement amounts conflict.'),
  }
  const issue = props.issue
  let amountLabel = t('Recorded consumption')
  if (issue.log_type === LOG_TYPE_ENUM.REFUND) {
    amountLabel = t('Recorded refund')
  }
  if (issue.kind === 'task_amount') amountLabel = t('Linked records net amount')
  return (
    <article
      aria-label={t('Billing item {{reference}}', { reference: issue.id })}
      className='space-y-3 rounded-md border p-4'
    >
      <div className='flex flex-wrap justify-between gap-2 font-medium'>
        <span>{issue.model || t('Unknown model')}</span>
        <span>
          {issue.blocking
            ? t('Unresolved amount difference')
            : t('Explanation gap')}
        </span>
      </div>
      <p className='text-muted-foreground text-sm'>
        {new Date(issue.created_at * 1000).toLocaleString()} · {amountLabel}:{' '}
        {formatCustomerStatementQuota(Number(issue.quota))}
      </p>
      <p className='text-sm'>
        {issue.blocking
          ? t(
              'The records do not yet explain the amount consistently. This is a review signal, not an amount to collect or refund.'
            )
          : t(
              'This record is already included in the statement. Historical billing details are incomplete, so the charge cannot be fully recalculated.'
            )}
      </p>
      <ul className='list-inside list-disc text-sm'>
        {issue.reasons.map((reason) => (
          <li key={reason}>
            {reasons[reason] || t('Evidence requires investigation.')}
          </li>
        ))}
      </ul>
      <Collapsible className='rounded border p-2 text-sm'>
        <CollapsibleTrigger render={<Button size='sm' variant='ghost' />}>
          {t('Technical details for investigation')}
        </CollapsibleTrigger>
        <CollapsibleContent>
          <p>
            {t('Related log IDs')}:{' '}
            {issue.related_log_ids.join(', ') || t('None')}
          </p>
          <p>
            {issue.id} · {issue.quota} quota
          </p>
          {issue.target_quota != null && (
            <p>
              {t('Task amount')}:{' '}
              {formatCustomerStatementQuota(Number(issue.target_quota))}.{' '}
              {t('This comparison is not an approved customer charge.')}
            </p>
          )}
          <ul className='space-y-2'>
            {issue.related_logs?.map((log) => (
              <li key={log.id}>
                #{log.id} · {new Date(log.created_at * 1000).toLocaleString()} ·{' '}
                {log.log_type === LOG_TYPE_ENUM.REFUND
                  ? t('Refund')
                  : t('Consume')}{' '}
                · {formatCustomerStatementQuota(Number(log.quota))}
                {!log.identity_matches && (
                  <span className='text-destructive'>
                    {' '}
                    · {t('Identity mismatch; excluded from comparison.')}
                  </span>
                )}
              </li>
            ))}
          </ul>
        </CollapsibleContent>
      </Collapsible>
      {issue.blocking && (
        <p className='text-destructive text-sm'>
          {t(
            'A note cannot clear this difference. Resolve it through the controlled correction process, then run the checks again.'
          )}
        </p>
      )}
      {issue.reviewed_at > 0 && (
        <p className='text-sm'>
          {issue.reviewed
            ? t('Reviewed; explanation remains incomplete.')
            : t('Investigation note recorded.')}{' '}
          {t('Reviewer {{actor}}, {{time}}', {
            actor: issue.actor_id,
            time: new Date(issue.reviewed_at * 1000).toLocaleString(),
          })}
          <br />
          {issue.note}
        </p>
      )}
      {props.disappeared && (
        <p role='alert'>
          {t(
            'This item no longer appears in the latest checks. Your unsaved input is kept for reference; discard it when you are ready.'
          )}
        </p>
      )}
      {props.canWrite && (
        <SourceDecisionForm
          issue={issue}
          disabled={props.disabled || props.disappeared}
          pendingRequest={props.pendingRequest}
          draft={props.draft}
          onDraftChange={props.onDraftChange}
          scope={props.scope}
          fingerprint={props.fingerprint}
          onSaved={props.onSaved}
        />
      )}
    </article>
  )
}
