import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import { formatCustomerStatementQuota } from '../lib'
import type {
  SourceDecisionRecord,
  SourceReviewScope,
} from '../source-review-api'
import {
  SourceDecisionForm,
  type SourceDecisionDraft,
} from './source-decision-form'
import { useSourceDecisionLabels } from './source-decision-labels'

export type SourceRequestDraft = {
  request: SourceDecisionRecord
  form: SourceDecisionDraft
}

export function SourcePendingRequests(props: {
  requests: SourceDecisionRecord[]
  scope: SourceReviewScope
  fingerprint: string
  canWrite: boolean
  disabled: boolean
  hasDrafts: boolean
  drafts: Record<string, SourceRequestDraft>
  onDraftChange: (
    request: SourceDecisionRecord,
    draft: SourceDecisionDraft | undefined
  ) => void
  onSaved: () => void
}) {
  const { t } = useTranslation()
  const labels = useSourceDecisionLabels()
  const [page, setPage] = useState(0)
  const pages = Math.max(1, Math.ceil(props.requests.length / 10))
  const current = Math.min(page, pages - 1)
  const pageRequests = props.requests.slice(current * 10, current * 10 + 10)
  const visibleRequests = [
    ...pageRequests,
    ...Object.values(props.drafts)
      .filter(
        ({ request }) =>
          !pageRequests.some((current) => current.issue_id === request.issue_id)
      )
      .map(
        ({ request }) =>
          props.requests.find(
            (current) => current.issue_id === request.issue_id
          ) ?? request
      ),
  ]
  if (!visibleRequests.length) return null
  return (
    <section className='space-y-3' aria-label={t('Open handling requests')}>
      <p className='font-medium'>{t('Open handling requests')}</p>
      <p>
        {t(
          'These requests still need a decision, even if the billing evidence has been corrected. Closing a request does not execute a refund or adjustment.'
        )}
      </p>
      {visibleRequests.map((request) => (
        <article
          className='space-y-3 rounded border p-4'
          key={request.issue_id}
          aria-label={t('Open request #{{id}}', { id: request.id })}
        >
          <p>
            {request.model} · {labels.decisions[request.decision]}
          </p>
          <p>
            {t('Recorded amount at the time')}:{' '}
            {formatCustomerStatementQuota(Number(request.recorded_quota))}
          </p>
          <p>
            {t('Administrator #{{actor}} · {{time}}', {
              actor: request.actor_id,
              time: new Date(request.created_at * 1000).toLocaleString(),
            })}
          </p>
          <p className='break-words whitespace-pre-wrap'>{request.note}</p>
          {!request.current_source && (
            <p>
              {t(
                'Billing evidence has changed; this request is still pending.'
              )}
            </p>
          )}
          {!props.requests.some(
            (current) => current.issue_id === request.issue_id
          ) && (
            <p role='alert'>
              {t(
                'This item no longer appears in the latest checks. Your unsaved input is kept for reference; discard it when you are ready.'
              )}
            </p>
          )}
          {props.canWrite && (
            <SourceDecisionForm
              issue={{
                id: request.issue_id,
                decision_id: request.id,
                log_type: request.log_type,
                blocking: request.blocking,
              }}
              scope={props.scope}
              fingerprint={props.fingerprint}
              closureOnly
              disabled={
                props.disabled ||
                !props.requests.some(
                  (current) => current.issue_id === request.issue_id
                )
              }
              draft={props.drafts[request.issue_id]?.form}
              onDraftChange={(draft) => props.onDraftChange(request, draft)}
              onSaved={props.onSaved}
            />
          )}
        </article>
      ))}
      {pages > 1 && (
        <div className='flex items-center gap-3'>
          <Button
            variant='outline'
            disabled={props.hasDrafts || current === 0}
            onClick={() => setPage(current - 1)}
          >
            {t('Previous requests')}
          </Button>
          <span>
            {current + 1} / {pages}
          </span>
          <Button
            variant='outline'
            disabled={props.hasDrafts || current + 1 >= pages}
            onClick={() => setPage(current + 1)}
          >
            {t('Next requests')}
          </Button>
        </div>
      )}
    </section>
  )
}
