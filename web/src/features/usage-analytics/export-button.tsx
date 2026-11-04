import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import { Field, FieldLabel } from '@/components/ui/field'
import { ExportJobsDrawer } from '@/features/billing-reconciliation/export-jobs-drawer'

import {
  createUsageAdminExport,
  createUsageSelfExport,
  type UsageExportBody,
} from './api'
import type { UsageAnalyticsPeriod, UsageCustomerView } from './types'
import { formatPeriodRange } from './utils'

type UsageExportButtonProps = {
  body: UsageExportBody
  period?: UsageAnalyticsPeriod
  customerView?: UsageCustomerView
  ready: boolean
}

export function UsageExportButton(props: UsageExportButtonProps) {
  // Changing the source scope starts a fresh form, including its selected model.
  return <UsageExportScope key={JSON.stringify(props.body)} {...props} />
}

function UsageExportScope(props: UsageExportButtonProps) {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)
  const [modelName, setModelName] = useState('')
  const [focusedJobId, setFocusedJobId] = useState<string>()
  const models = [
    ...new Set(
      props.customerView?.keys.flatMap((key) =>
        key.models.map((model) => model.model_name)
      ) ?? []
    ),
  ].sort()
  const customerScope =
    props.body.view === 'self' || props.body.view === 'customer'
  let scopeLabel = t('Customer usage')
  if (props.body.view === 'self') scopeLabel = t('My usage')
  if (props.body.view === 'upstream') scopeLabel = t('Upstream usage')
  if (props.body.view === 'customer') {
    scopeLabel = t('Customer #{{id}}', { id: props.body.user_id })
  }
  const mutation = useMutation({
    mutationFn: async () => {
      const body = {
        ...props.body,
        language: i18n.language.startsWith('zh') ? 'zh' : 'en',
        model_name: customerScope && modelName ? modelName : undefined,
      }
      const response =
        props.body.view === 'self'
          ? await createUsageSelfExport(body)
          : await createUsageAdminExport(body)
      return response.data
    },
    onSuccess: (job) => {
      queryClient.setQueryData(['customer-export-job', job.job_id], job)
      setFocusedJobId(job.job_id)
      void queryClient.invalidateQueries({ queryKey: ['customer-export-jobs'] })
    },
  })

  return (
    <>
      <Button size='sm' onClick={() => setOpen(true)}>
        {t('Export and download')}
      </Button>
      <ExportJobsDrawer
        open={open}
        onOpenChange={setOpen}
        focusedJobId={focusedJobId}
      >
        <section
          className='space-y-3 rounded-lg border p-3'
          aria-label={t('New usage export')}
        >
          <h2 className='text-sm font-medium'>{t('New usage export')}</h2>
          <p className='text-muted-foreground text-xs'>
            {props.period
              ? formatPeriodRange(props.period)
              : `${t(props.body.period === 'week' ? 'Week' : 'Day')} · ${props.body.date}`}{' '}
            · Asia/Shanghai
          </p>
          <p className='text-sm'>{scopeLabel}</p>
          {props.body.search && (
            <p className='text-muted-foreground text-xs break-all'>
              {t('Search customer')}: {props.body.search}
            </p>
          )}
          {customerScope && (
            <Field>
              <FieldLabel htmlFor='usage-export-model'>
                {t('Export model')}
              </FieldLabel>
              <Combobox
                id='usage-export-model'
                aria-label={t('Export model')}
                value={modelName}
                disabled={!props.ready || mutation.isPending}
                options={[
                  { value: '', label: t('All models') },
                  ...models.map((name) => ({ value: name, label: name })),
                ]}
                onValueChange={(value) => setModelName(value ?? '')}
                className='w-full'
              />
            </Field>
          )}
          <p className='text-muted-foreground text-xs'>
            {t(
              'Generate a new file for this scope, or download an existing export below.'
            )}
          </p>
          <Button
            size='sm'
            disabled={
              !props.ready ||
              mutation.isPending ||
              (modelName !== '' && !models.includes(modelName))
            }
            onClick={() => mutation.mutate()}
          >
            {mutation.isPending ? t('Submitting...') : t('Generate CSV')}
          </Button>
          {mutation.isError && (
            <p role='alert' className='text-destructive text-sm'>
              {t(mutation.error.message)}
            </p>
          )}
        </section>
      </ExportJobsDrawer>
    </>
  )
}
