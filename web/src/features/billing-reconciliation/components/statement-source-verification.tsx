import { useMutation, useQuery } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { ReportCard } from '@/components/report-card'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { toIntlLocale } from '@/i18n/languages'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { formatBillingMonth } from '../lib'
import { getSourceReview, type SourceIssue } from '../source-review-api'
import { formatShanghaiTimestamp } from '../upstream-reconciliation-utils'
import { verifyAdminStatementSource } from '../version-api'
import { BillingPagination } from './billing-pagination'
import type { SourceDecisionDraft } from './source-decision-form'
import { SourceDecisionHistory } from './source-decision-history'
import {
  SourcePendingRequests,
  type SourceRequestDraft,
} from './source-pending-requests'
import { SourceReviewError } from './source-review-error'
import { SourceReviewIssue } from './source-review-issue'

export function StatementSourceVerification(props: {
  userId: number
  period: { start_timestamp: number; end_timestamp: number }
  statementDataCutoff?: number
  disabled: boolean
  onVerified: () => void
}) {
  const { t, i18n } = useTranslation()
  const isRoot = useAuthStore(
    (state) => state.auth.user?.role === ROLE.SUPER_ADMIN
  )
  const [open, setOpen] = useState(false)
  const [issueDrafts, setIssueDrafts] = useState<
    Record<string, { issue: SourceIssue; form: SourceDecisionDraft }>
  >({})
  const [requestDrafts, setRequestDrafts] = useState<
    Record<string, SourceRequestDraft>
  >({})
  const hasDrafts =
    Object.keys(issueDrafts).length > 0 || Object.keys(requestDrafts).length > 0
  const [page, setPage] = useState(0)
  const [pageSize, setPageSize] = useState(10)
  const [backup, setBackup] = useState('')
  const [retention, setRetention] = useState('')
  const id = useId()
  const scope = { ...props.period, user_id: props.userId }
  const query = useQuery({
    queryKey: [
      'billing-source-review',
      props.userId,
      props.period.start_timestamp,
      props.period.end_timestamp,
    ],
    queryFn: async ({ signal }) => {
      const result = await getSourceReview(scope, signal)
      if (
        !result.success ||
        !result.data ||
        result.data.decision_version !== 3
      ) {
        throw new Error(
          result.message ||
            t('Source checks failed. No completeness conclusion is available.')
        )
      }
      return result.data
    },
    enabled: open && !props.disabled,
    retry: false,
    meta: { errorToast: false, redirectOnInternalServerError: false },
    staleTime: 0,
    refetchOnWindowFocus: false,
  })
  const report = query.data
  const locale = toIntlLocale(i18n.language)
  const formatCount = new Intl.NumberFormat(locale).format
  const issues = (report?.issues ?? []).filter(
    (issue) => !issue.blocking || !issue.reviewed
  )
  const hasFindings = Boolean(report && (report.blockers || report.pending))
  const monthEnded = Date.now() >= (props.period.end_timestamp + 1) * 1000
  let statementStatus = t(
    'Record source completeness before issuing the statement.'
  )
  if (!monthEnded) {
    statementStatus = t('The billing month is still open.')
  } else if (report?.retention_status === 'partial') {
    statementStatus = t('Source logs partially cleaned; correction is blocked.')
  } else if (hasFindings) {
    statementStatus = t('Resolve pending issues before issuing the statement.')
  } else if (report?.retention_needs_review) {
    statementStatus = t('Verify source completeness again.')
  } else if (['intact', 'none'].includes(report?.retention_status ?? '')) {
    statementStatus = t(
      'Source checks complete. Continue with the statement workflow.'
    )
  }
  const mutation = useMutation({
    meta: { errorToast: false },
    mutationFn: async () => {
      if (!report) throw new Error('No source report')
      const result = await verifyAdminStatementSource(scope, {
        fingerprint: report.fingerprint,
        backup_evidence: backup.trim(),
        retention_evidence: retention.trim(),
      })
      if (!result.success) {
        throw new Error(
          result.message ||
            t('Verification was rejected. Refresh the checks before retrying.')
        )
      }
    },
    onSuccess: () => {
      setOpen(false)
      setBackup('')
      setRetention('')
      toast.success(t('Source verification recorded.'))
      props.onVerified()
    },
  })
  const pageCount = Math.max(1, Math.ceil(issues.length / pageSize))
  const currentPage = Math.min(page, pageCount - 1)
  const pageIssues = issues.slice(
    currentPage * pageSize,
    (currentPage + 1) * pageSize
  )
  // Drafts remain visible even when a refresh moves their record off this page.
  const visibleIssues = [
    ...pageIssues,
    ...Object.values(issueDrafts)
      .filter(
        ({ issue }) => !pageIssues.some((current) => current.id === issue.id)
      )
      .map(
        ({ issue }) =>
          issues.find((current) => current.id === issue.id) ?? issue
      ),
  ]
  const blocked =
    !isRoot ||
    hasDrafts ||
    props.disabled ||
    !report ||
    query.isFetching ||
    query.isError ||
    report.blockers > 0 ||
    report.pending > 0 ||
    !backup.trim() ||
    !retention.trim()
  return (
    <>
      <Button
        size='sm'
        variant='outline'
        disabled={props.disabled}
        onClick={() => {
          setPage(0)
          setOpen(true)
        }}
      >
        {t('Check statement records')}
      </Button>
      <Dialog
        open={open}
        onOpenChange={(next) => {
          if (!mutation.isPending && !hasDrafts) setOpen(next)
        }}
        title={t('Check statement records')}
        contentClassName='sm:max-w-4xl'
        description={`${t('Customer #{{id}}', { id: props.userId })} · ${formatBillingMonth(props.period.start_timestamp, locale)}`}
        footer={
          <Button
            variant='outline'
            onClick={() => setOpen(false)}
            disabled={mutation.isPending || hasDrafts}
          >
            {t('Close')}
          </Button>
        }
      >
        <div className='space-y-4'>
          <div className='flex flex-wrap items-start justify-between gap-3'>
            <p className='text-muted-foreground max-w-prose text-sm'>
              {t(
                'Checks the customer’s records for this billing month. The statement may have a different data cutoff.'
              )}
            </p>
            <Button
              variant='outline'
              size='sm'
              disabled={query.isFetching || mutation.isPending}
              onClick={() => void query.refetch()}
            >
              {t('Run checks again')}
            </Button>
          </div>
          {query.isFetching && (
            <p role='status'>{t('Checking the full scope. Please wait.')}</p>
          )}
          {query.isError && <SourceReviewError error={query.error} />}
          {report && (!query.isError || hasDrafts || backup || retention) && (
            <>
              {!query.isFetching && !query.isError && (
                <ReportCard className='gap-1' aria-live='polite'>
                  <h3 className='text-sm font-semibold'>
                    {hasFindings
                      ? t('Some records need review')
                      : t('No issues found in the available records')}
                  </h3>
                  <p className='text-sm'>
                    {t('{{rows}} charge and refund records checked', {
                      rows: formatCount(report.rows),
                    })}
                    {report.pending > 0 && (
                      <>
                        {' '}
                        ·{' '}
                        {t('{{count}} awaiting action', {
                          count: report.pending,
                        })}
                      </>
                    )}
                  </p>
                  <p className='text-sm font-medium'>{statementStatus}</p>
                  {!hasFindings && (
                    <p className='text-muted-foreground text-sm'>
                      {t(
                        'This check can only see records that are still available. Verifying historical coverage requires checking backups and log retention separately.'
                      )}
                    </p>
                  )}
                  <p className='text-muted-foreground text-xs'>
                    {t('Checked at {{time}}', {
                      time: formatShanghaiTimestamp(report.captured_at),
                    })}
                  </p>
                  {!!props.statementDataCutoff && (
                    <p className='text-muted-foreground text-xs'>
                      {t('Data as of {{time}}', {
                        time: formatShanghaiTimestamp(
                          props.statementDataCutoff
                        ),
                      })}
                    </p>
                  )}
                </ReportCard>
              )}
              {hasFindings && (
                <p className='text-muted-foreground text-sm'>
                  {t(
                    'Review the items below. Saving a decision records it in the handling history. Any refund or correction must be completed separately.'
                  )}
                </p>
              )}
              {(report.audit_issues || []).length > 0 && (
                <div role='alert' className='text-destructive'>
                  <p>
                    {t(
                      'Some handling records are damaged. You can view the report, but decisions and verification are blocked. Ask the technical reviewer to recover the audit history without deleting records.'
                    )}
                  </p>
                  <ul>
                    {report.audit_issues.map((row) => (
                      <li key={row.id}>
                        {t(
                          'Audit record #{{id}} · administrator #{{actor}} · {{time}}',
                          {
                            id: row.id,
                            actor: row.actor_id,
                            time: new Date(
                              row.created_at * 1000
                            ).toLocaleString(),
                          }
                        )}
                      </li>
                    ))}
                  </ul>
                </div>
              )}
              {hasDrafts && (
                <p role='status'>
                  {t(
                    'You have unsaved input. Save or discard it before changing pages or closing this window.'
                  )}
                </p>
              )}
              <SourcePendingRequests
                requests={report.pending_requests || []}
                scope={scope}
                fingerprint={report.fingerprint}
                canWrite={isRoot}
                disabled={
                  props.disabled ||
                  query.isFetching ||
                  query.isError ||
                  mutation.isPending ||
                  (report.audit_issues || []).length > 0
                }
                hasDrafts={hasDrafts}
                drafts={requestDrafts}
                onDraftChange={(request, form) =>
                  setRequestDrafts((prev) => {
                    if (form) {
                      return { ...prev, [request.issue_id]: { request, form } }
                    }
                    if (!prev[request.issue_id]) return prev
                    const next = { ...prev }
                    delete next[request.issue_id]
                    return next
                  })
                }
                onSaved={() => {
                  void query.refetch()
                  props.onVerified()
                }}
              />
              {issues.some((issue) => issue.blocking) && (
                <p role='alert' className='text-destructive'>
                  {t('Resolve blocking issues before recording completeness.')}
                </p>
              )}
              {visibleIssues.map((issue) => (
                <SourceReviewIssue
                  key={issue.id}
                  issue={issue}
                  scope={scope}
                  fingerprint={report.fingerprint}
                  canWrite={isRoot}
                  disabled={
                    props.disabled ||
                    query.isFetching ||
                    query.isError ||
                    mutation.isPending ||
                    (report.audit_issues || []).length > 0
                  }
                  pendingRequest={(report.pending_requests || []).some(
                    (request) => request.issue_id === issue.id
                  )}
                  draft={issueDrafts[issue.id]?.form}
                  onDraftChange={(form) =>
                    setIssueDrafts((prev) => {
                      if (form) return { ...prev, [issue.id]: { issue, form } }
                      if (!prev[issue.id]) return prev
                      const next = { ...prev }
                      delete next[issue.id]
                      return next
                    })
                  }
                  disappeared={
                    !issues.some((current) => current.id === issue.id)
                  }
                  onSaved={() => {
                    void query.refetch()
                    props.onVerified()
                  }}
                />
              ))}
              {issues.length > 0 && (
                <BillingPagination
                  page={currentPage + 1}
                  pageSize={pageSize}
                  total={issues.length}
                  label={t('Check statement records')}
                  disabled={hasDrafts}
                  onChange={(nextPage, nextSize) => {
                    setPage(nextPage - 1)
                    setPageSize(nextSize)
                  }}
                />
              )}
              {!!report.history?.length && (
                <SourceDecisionHistory history={report.history} />
              )}
              {isRoot ? (
                <Collapsible className='space-y-2 border-t pt-4'>
                  <CollapsibleTrigger render={<Button variant='outline' />}>
                    {t('Technical source verification')}
                  </CollapsibleTrigger>
                  <CollapsibleContent className='space-y-2'>
                    <p className='text-muted-foreground text-sm'>
                      {t(
                        'Verify this month’s coverage using backup, log retention and recovery records.'
                      )}
                    </p>
                    <Label htmlFor={`${id}-backup`}>
                      {t('Backup and coverage evidence')}
                    </Label>
                    <Textarea
                      id={`${id}-backup`}
                      value={backup}
                      maxLength={4000}
                      onChange={(e) => setBackup(e.target.value)}
                      placeholder={t(
                        'Identify the backup or source, covered dates, and how missing records were checked.'
                      )}
                    />
                    <Label htmlFor={`${id}-retention`}>
                      {t('Log retention and recovery evidence')}
                    </Label>
                    <Textarea
                      id={`${id}-retention`}
                      value={retention}
                      maxLength={4000}
                      onChange={(e) => setRetention(e.target.value)}
                      placeholder={t(
                        'Record checks of log cleanup, write failures and recovery, including any remaining uncertainty.'
                      )}
                    />
                    <p className='text-muted-foreground text-sm'>
                      {t(
                        'Only record completeness when both sources support it. Text entry does not repair missing data. Pending versions for this month must be regenerated; confirmed versions are preserved.'
                      )}
                    </p>
                    {isRoot && !query.isError && (
                      <Button
                        disabled={blocked || mutation.isPending}
                        onClick={() => mutation.mutate()}
                      >
                        {t('Record verified completeness')}
                      </Button>
                    )}
                  </CollapsibleContent>
                </Collapsible>
              ) : (
                <p>
                  {t(
                    'A super administrator must verify statement sources first.'
                  )}
                </p>
              )}
            </>
          )}
          {mutation.isError && (
            <SourceReviewError
              error={mutation.error}
              operation='verification'
            />
          )}
        </div>
      </Dialog>
    </>
  )
}
