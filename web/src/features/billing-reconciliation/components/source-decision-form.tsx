import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'
import { LOG_TYPE_ENUM } from '@/features/usage-logs/constants'

import {
  recordSourceReview,
  type SourceDecision,
  type SourceDecisionInput,
  type SourceIssue,
  type SourceReviewScope,
} from '../source-review-api'
import { useSourceDecisionLabels } from './source-decision-labels'

const schema = z
  .object({
    decision: z.enum([
      'keep_record',
      'investigate',
      'request_waiver',
      'request_duplicate_exclusion',
      'request_period_correction',
      'withdraw_request',
      'reject_request',
    ]),
    reason: z.string().min(1),
    note: z.string().trim().max(4000),
  })
  .refine((v) => v.decision === 'keep_record' || v.note.length > 0, {
    path: ['note'],
    message: 'required',
  })
const reasons = {
  keep_record: ['accept_missing_details'],
  investigate: ['amount_questioned', 'missing_evidence'],
  request_waiver: ['customer_agreement', 'service_issue'],
  request_duplicate_exclusion: ['suspected_duplicate'],
  request_period_correction: ['wrong_period'],
  withdraw_request: ['request_withdrawn'],
  reject_request: ['request_rejected'],
} as const

export type SourceDecisionDraft = {
  values: Pick<SourceDecisionInput, 'decision' | 'reason' | 'note'>
  reviewedContext: string
}

export function SourceDecisionForm(props: {
  issue: Pick<SourceIssue, 'id' | 'decision_id' | 'blocking' | 'log_type'>
  disabled?: boolean
  closureOnly?: boolean
  pendingRequest?: boolean
  draft?: SourceDecisionDraft
  onDraftChange: (draft: SourceDecisionDraft | undefined) => void
  scope: SourceReviewScope
  fingerprint: string
  onSaved: () => void
}) {
  const { t } = useTranslation()
  const labels = useSourceDecisionLabels()
  const requiredNoteLabel = props.closureOnly
    ? t('Reason for closing this request')
    : t('What needs to be reviewed?')
  const emptyValues: SourceDecisionDraft['values'] = {
    decision: props.closureOnly ? 'withdraw_request' : 'investigate',
    reason: props.closureOnly ? 'request_withdrawn' : 'missing_evidence',
    note: '',
  }
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: props.draft?.values ?? emptyValues,
  })
  const fieldID = `${props.closureOnly ? 'request' : 'issue'}-${props.issue.id}`
  const decision = form.watch('decision')
  const reason = form.watch('reason')
  const note = form.watch('note')
  const context = `${props.fingerprint}:${props.issue.decision_id || 0}`
  const [reviewedContext, setReviewedContext] = useState(
    props.draft?.reviewedContext ?? context
  )
  const dirty =
    decision !== emptyValues.decision ||
    reason !== emptyValues.reason ||
    note !== ''
  const needsReview = dirty && reviewedContext !== context
  const draftCallback = useRef(props.onDraftChange)
  draftCallback.current = props.onDraftChange
  useEffect(() => {
    draftCallback.current(
      dirty
        ? { values: { decision, reason, note }, reviewedContext }
        : undefined
    )
    // Unmounting or moving between pages must not discard a draft.
  }, [dirty, decision, reason, note, reviewedContext])
  const retry = useRef<{ key: string; body: SourceDecisionInput } | null>(null)
  const mutation = useMutation({
    mutationFn: async (body: SourceDecisionInput) => {
      const result = await recordSourceReview(props.scope, body)
      if (!result.success || result.decision_version !== 3) {
        throw new Error('Source decision rejected')
      }
    },
    onSuccess: () => {
      form.reset(emptyValues)
      props.onDraftChange(undefined)
      setReviewedContext(context)
      props.onSaved()
    },
  })
  const submit = form.handleSubmit((values) => {
    if (props.disabled || needsReview || mutation.isPending) return
    const base = {
      ...values,
      fingerprint: props.fingerprint,
      issue_id: props.issue.id,
      previous_id: props.issue.decision_id || 0,
    }
    const key = JSON.stringify(base)
    if (!retry.current || retry.current.key !== key) {
      retry.current = {
        key,
        body: { ...base, request_id: crypto.randomUUID() },
      }
    }
    mutation.mutate(retry.current.body)
  })
  return (
    <form onSubmit={submit} className='space-y-3'>
      <FieldGroup>
        <Field>
          <FieldLabel htmlFor={`decision-${fieldID}`}>
            {t('Handling decision')}
          </FieldLabel>
          <NativeSelect
            id={`decision-${fieldID}`}
            value={decision}
            disabled={mutation.isPending || props.disabled}
            onChange={(e) => {
              const next = e.target.value as SourceDecision
              form.setValue('decision', next, { shouldDirty: true })
              form.setValue('reason', reasons[next][0], { shouldDirty: true })
            }}
          >
            {Object.entries(labels.decisions)
              .filter(([value]) =>
                props.closureOnly
                  ? value === 'withdraw_request' || value === 'reject_request'
                  : value !== 'withdraw_request' && value !== 'reject_request'
              )
              .map(([value, label]) => (
                <NativeSelectOption
                  key={value}
                  value={value}
                  disabled={
                    (value === 'keep_record' &&
                      (props.issue.blocking || props.pendingRequest)) ||
                    (!props.closureOnly &&
                      props.issue.log_type === LOG_TYPE_ENUM.REFUND &&
                      value !== 'keep_record' &&
                      value !== 'investigate')
                  }
                >
                  {label}
                </NativeSelectOption>
              ))}
          </NativeSelect>
        </Field>
        <Field>
          <FieldLabel htmlFor={`reason-${fieldID}`}>
            {t('Business reason')}
          </FieldLabel>
          <NativeSelect
            id={`reason-${fieldID}`}
            {...form.register('reason')}
            disabled={mutation.isPending || props.disabled}
          >
            {reasons[decision].map((value) => (
              <NativeSelectOption key={value} value={value}>
                {labels.reasons[value]}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </Field>
        <Field data-invalid={!!form.formState.errors.note}>
          <FieldLabel htmlFor={`note-${fieldID}`}>
            {decision === 'keep_record'
              ? t('Additional note (optional)')
              : requiredNoteLabel}
          </FieldLabel>
          <Textarea
            id={`note-${fieldID}`}
            {...form.register('note')}
            maxLength={4000}
            disabled={mutation.isPending || props.disabled}
            aria-invalid={!!form.formState.errors.note}
            placeholder={
              props.closureOnly
                ? t(
                    'Explain why the request is withdrawn or rejected, and who agreed to keep the existing accounting.'
                  )
                : t(
                    'Describe the customer request or concern. Technical proof is not required here.'
                  )
            }
          />
          {form.formState.errors.note && (
            <p role='alert'>
              {t('Describe the reason for this review request.')}
            </p>
          )}
        </Field>
      </FieldGroup>
      <div className='bg-muted rounded-md p-3 text-sm' aria-live='polite'>
        <p className='font-medium'>{t('Effect of saving this decision')}</p>
        <p>
          {t(
            'Statement adjustment: $0. Customer balance change: $0. No additional charge or refund.'
          )}
        </p>
        <p>
          {props.closureOnly &&
            t(
              'This closes the request without changing charges or refunds. Any remaining billing issue still needs review.'
            )}
          {!props.closureOnly &&
            (decision === 'keep_record'
              ? t(
                  'The existing charge or refund stays on the statement. Missing details remain disclosed; this does not prove the amount is correct.'
                )
              : t(
                  'This saves a pending request only. No fee is waived, no record is excluded, and no refund is executed. Review remains incomplete.'
                ))}
        </p>
        <p>
          {t(
            'The system records your identity, time, decision and amount automatically. Earlier decisions remain in the history.'
          )}
        </p>
      </div>
      {needsReview && (
        <div role='alert' className='text-sm'>
          <p>
            {t(
              'The records changed. Your input is preserved. Review the latest evidence before submitting.'
            )}
          </p>
          <Button
            type='button'
            variant='outline'
            disabled={props.disabled}
            onClick={() => setReviewedContext(context)}
          >
            {t('I have reviewed the updated records')}
          </Button>
        </div>
      )}
      <Button
        type='submit'
        disabled={mutation.isPending || props.disabled || needsReview}
      >
        {t('Save handling decision')}
      </Button>
      {dirty && (
        <Button
          type='button'
          variant='ghost'
          disabled={mutation.isPending}
          onClick={() => {
            form.reset(emptyValues)
            props.onDraftChange(undefined)
            setReviewedContext(context)
          }}
        >
          {t('Discard unsaved changes')}
        </Button>
      )}
      {mutation.isError && (
        <p role='alert' className='text-destructive text-sm'>
          {t(
            'Unable to save the decision. Refresh to check for changes before retrying; do not assume it was completed.'
          )}
        </p>
      )}
    </form>
  )
}
