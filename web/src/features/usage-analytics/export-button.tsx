import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ReportCard } from '@/components/report-card'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import { Field, FieldLabel } from '@/components/ui/field'
import {
  downloadExportFile,
  requestExportDownload,
} from '@/features/billing-reconciliation/export-api'
import { ExportJobsDrawer } from '@/features/billing-reconciliation/export-jobs-drawer'
import { toIntlLocale } from '@/i18n/languages'

import {
  createUsageAdminExport,
  createUsageSelfExport,
  type UsageExportBody,
} from './api'
import type {
  UsageAnalyticsPeriod,
  UsageCustomerKeyGroup,
  UsageCustomerView,
} from './types'
import { formatPeriodRange, usageKeyLabel } from './utils'

type UsageExportButtonProps = {
  body: UsageExportBody
  period?: UsageAnalyticsPeriod
  customerView?: UsageCustomerView
  apiKey?: UsageCustomerKeyGroup
  customerName?: string
  upstreamURLName?: string
  upstreamChannelName?: string
  snapshotAt?: number
  triggerLabel?: string
  ready: boolean
}

export function UsageExportButton(props: UsageExportButtonProps) {
  // Changing the source scope starts a fresh form, including its selected model.
  const {
    view,
    period,
    date,
    token_id,
    user_id,
    search,
    model_name,
    url_key,
    channel_id,
  } = props.body
  const scope = JSON.stringify([
    view,
    period,
    date,
    token_id,
    user_id,
    search,
    model_name,
    url_key,
    channel_id,
  ])
  return <UsageExportScope key={scope} {...props} />
}

function UsageExportScope(props: UsageExportButtonProps) {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const formTitleId = useId()
  const [open, setOpen] = useState(false)
  const [modelName, setModelName] = useState(props.body.model_name ?? '')
  const [focusedJobId, setFocusedJobId] = useState<string>()
  const personalUsage = props.body.view === 'self'
  const adminCustomerUsage =
    props.body.view === 'customers' || props.body.view === 'customer'
  const reuseExports = personalUsage || adminCustomerUsage
  const periodLabels = { day: t('Day'), week: t('Week'), month: t('Monthly') }
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
  if (props.body.view === 'customers') {
    scopeLabel = props.body.search
      ? t('Customer search results')
      : t('All customers')
  }
  if (props.body.view === 'upstream') scopeLabel = t('Upstream usage')
  if (props.body.view === 'customer') {
    scopeLabel = t('Customer #{{id}}', { id: props.body.user_id })
    if (props.customerName) {
      scopeLabel = `${props.customerName} · #${props.body.user_id}`
    }
  }
  const mutation = useMutation({
    mutationFn: async () => {
      const body = {
        ...props.body,
        language: i18n.language.startsWith('zh') ? 'zh' : 'en',
        model_name: customerScope && modelName ? modelName : undefined,
      }
      const response = personalUsage
        ? await createUsageSelfExport(body)
        : await createUsageAdminExport(body)
      return response.data
    },
    onSuccess: async (job) => {
      queryClient.setQueryData(['customer-export-job', job.job_id], job)
      setFocusedJobId(job.job_id)
      void queryClient.invalidateQueries({
        queryKey: ['customer-export-jobs'],
      })
      if (
        reuseExports &&
        job.status === 'succeeded' &&
        job.artifact?.files.length === 1
      ) {
        try {
          const download = await requestExportDownload(job.job_id)
          if (download.files.length !== 1) {
            throw new Error('Export file unavailable')
          }
          downloadExportFile(download.files[0].url)
        } catch {
          toast.error(t('Unable to download export.'))
        }
      }
    },
  })
  let submitLabel = reuseExports ? t('Export CSV') : t('Generate CSV')
  if (mutation.isPending) submitLabel = t('Submitting...')

  return (
    <>
      <Button size='sm' onClick={() => setOpen(true)}>
        {props.triggerLabel ?? t('Export and download')}
      </Button>
      <ExportJobsDrawer
        open={open}
        onOpenChange={(nextOpen) => {
          setOpen(nextOpen)
          if (!nextOpen && reuseExports) setFocusedJobId(undefined)
        }}
        focusedJobId={focusedJobId}
        personalUsage={personalUsage}
        adminCustomerUsage={adminCustomerUsage}
      >
        <ReportCard role='region' aria-labelledby={formTitleId}>
          <h2 id={formTitleId} className='text-sm font-medium'>
            {reuseExports
              ? t('Export usage statistics')
              : t('New usage export')}
          </h2>
          <p className='text-muted-foreground text-xs'>
            {props.period
              ? formatPeriodRange(props.period)
              : `${periodLabels[props.body.period]} · ${props.body.date}`}{' '}
            · {reuseExports ? t('Shanghai time') : 'Asia/Shanghai'}
          </p>
          {!personalUsage && (
            <p className='text-sm font-medium wrap-anywhere'>{scopeLabel}</p>
          )}
          {props.body.view === 'upstream' && (
            <dl className='text-muted-foreground space-y-1 text-xs'>
              <div>
                <dt className='font-medium'>{t('Upstream URL')}</dt>
                <dd className='wrap-anywhere'>
                  {props.upstreamURLName ?? t('All upstream URLs')}
                </dd>
              </div>
              <div>
                <dt className='font-medium'>{t('Channel')}</dt>
                <dd className='wrap-anywhere'>
                  {props.upstreamChannelName ?? t('All channels')}
                </dd>
              </div>
              {!!props.snapshotAt && props.period && (
                <div>
                  <dt className='font-medium'>{t('Report snapshot')}</dt>
                  <dd>
                    {new Intl.DateTimeFormat(toIntlLocale(i18n.language), {
                      dateStyle: 'short',
                      timeStyle: 'short',
                      timeZone: props.period.timezone,
                    }).format(props.snapshotAt * 1000)}
                  </dd>
                </div>
              )}
            </dl>
          )}
          {props.apiKey && (
            <p className='text-muted-foreground text-xs break-all'>
              {t('API key')}: {usageKeyLabel(t, props.apiKey)}
            </p>
          )}
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
            {reuseExports
              ? t(
                  'We reuse an available export when the selected scope and usage report match.'
                )
              : t(
                  'Generate a new file for this scope, or download an existing export below.'
                )}
          </p>
          <Button
            size='sm'
            className='self-start'
            disabled={
              !props.ready ||
              mutation.isPending ||
              (modelName !== '' && !models.includes(modelName))
            }
            onClick={() => mutation.mutate()}
          >
            {submitLabel}
          </Button>
          {mutation.isError && (
            <p role='alert' className='text-destructive text-sm'>
              {t(mutation.error.message)}
            </p>
          )}
        </ReportCard>
      </ExportJobsDrawer>
    </>
  )
}
