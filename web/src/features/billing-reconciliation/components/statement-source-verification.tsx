import { useMutation, useQuery } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { getSourceReview, type SourceIssue } from '../source-review-api'
import { verifyAdminStatementSource } from '../version-api'
import type { SourceDecisionDraft } from './source-decision-form'
import { SourceDecisionHistory } from './source-decision-history'
import {
  SourcePendingRequests,
  type SourceRequestDraft,
} from './source-pending-requests'
import { SourceReviewIssue } from './source-review-issue'

export function StatementSourceVerification(props: {
  userId: number
  period: { start_timestamp: number; end_timestamp: number }
  disabled: boolean
  onVerified: () => void
}) {
  const { t } = useTranslation()
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
    queryFn: async () => {
      const result = await getSourceReview(scope)
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
    staleTime: 0,
    refetchOnWindowFocus: false,
  })
  const report = query.data
  const mutation = useMutation({
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
  const pageCount = Math.max(1, Math.ceil((report?.issues.length || 0) / 10))
  const currentPage = Math.min(page, pageCount - 1)
  const pageIssues =
    report?.issues.slice(currentPage * 10, currentPage * 10 + 10) || []
  // Drafts remain visible even when a refresh moves their record off this page.
  const visibleIssues = [
    ...pageIssues,
    ...Object.values(issueDrafts)
      .filter(
        ({ issue }) => !pageIssues.some((current) => current.id === issue.id)
      )
      .map(
        ({ issue }) =>
          report?.issues.find((current) => current.id === issue.id) ?? issue
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
        {t('Review and differences')}
      </Button>
      <Dialog
        open={open}
        onOpenChange={(next) => {
          if (!mutation.isPending && !hasDrafts) setOpen(next)
        }}
        title={t('Review and differences')}
        contentClassName='sm:max-w-4xl'
        description={t(
          'Customer #{{user}} · {{month}}. Choose how to handle each item; saving a decision does not move money.',
          {
            user: props.userId,
            month: new Date(
              props.period.start_timestamp * 1000
            ).toLocaleDateString(),
          }
        )}
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
          <p className='text-muted-foreground text-sm'>
            {t(
              'Review recorded amounts, choose a handling decision, and track it in the history. Requests stay pending until the actual correction is completed; this page does not execute refunds or remove charges.'
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
          {query.isFetching && (
            <p role='status'>{t('Checking the full scope. Please wait.')}</p>
          )}
          {query.isError && (
            <p role='alert' className='text-destructive'>
              {t(
                'Source checks failed. No completeness conclusion is available.'
              )}
            </p>
          )}
          {report && (
            <>
              <p>
                {t(
                  '{{rows}} records · {{issues}} review items · {{blockers}} blocking items · {{pending}} pending review',
                  {
                    rows: report.rows,
                    issues: report.issues.length,
                    blockers: report.blockers,
                    pending: report.pending,
                  }
                )}
              </p>
              <p className='text-muted-foreground text-xs'>
                {t('Checked at {{time}}', {
                  time: new Date(report.captured_at * 1000).toLocaleString(),
                })}
              </p>
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
              {report.issues.some((issue) => issue.blocking) && (
                <p role='alert' className='text-destructive'>
                  {t(
                    'Resolve amount differences before recording completeness. Review notes do not change charges or clear blocking items.'
                  )}
                </p>
              )}
              {report.issues.length === 0 && (
                <p>
                  {t(
                    'No issues found within the checked scope. This alone does not prove historical completeness.'
                  )}
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
                    !report.issues.some((current) => current.id === issue.id)
                  }
                  onSaved={() => {
                    void query.refetch()
                    props.onVerified()
                  }}
                />
              ))}
              {pageCount > 1 && (
                <div className='flex items-center gap-3'>
                  <Button
                    size='sm'
                    variant='outline'
                    disabled={currentPage === 0 || hasDrafts}
                    onClick={() => setPage(currentPage - 1)}
                  >
                    {t('Previous')}
                  </Button>
                  <span>
                    {currentPage + 1} / {pageCount}
                  </span>
                  <Button
                    size='sm'
                    variant='outline'
                    disabled={currentPage + 1 >= pageCount || hasDrafts}
                    onClick={() => setPage(currentPage + 1)}
                  >
                    {t('Next')}
                  </Button>
                </div>
              )}
              <SourceDecisionHistory history={report.history || []} />
              {isRoot ? (
                <Collapsible className='space-y-2 border-t pt-4'>
                  <CollapsibleTrigger render={<Button variant='outline' />}>
                    {t('Technical source verification')}
                  </CollapsibleTrigger>
                  <CollapsibleContent className='space-y-2'>
                    <p className='text-muted-foreground text-sm'>
                      {t(
                        'For the technical reviewer: check historical data coverage. Business managers do not need to invent backup evidence. Business decisions cannot replace this check.'
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
            <p role='alert' className='text-destructive'>
              {t(
                'Verification was rejected. Refresh the checks before retrying.'
              )}
            </p>
          )}
        </div>
      </Dialog>
    </>
  )
}
