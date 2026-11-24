import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  downloadExportArchive,
  downloadExportFile,
  hasExportCoverageFile,
  requestExportDownload,
  type CustomerExportJobView,
} from './export-api'

export function useBillingExportJob<T = void>(
  submit: (input: T) => Promise<CustomerExportJobView>
) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [focusedJobId, setFocusedJobId] = useState<string>()
  const [confirmation, setConfirmation] = useState<{
    job: CustomerExportJobView
    input: T
  }>()
  const existingDownload = useMutation({
    mutationFn: async (job: CustomerExportJobView) => {
      const download = await requestExportDownload(job.job_id, true)
      if (download.files.length > 1 || hasExportCoverageFile(job)) {
        await downloadExportArchive(job.job_id)
      } else if (download.files.length === 1) {
        downloadExportFile(download.files[0].url)
      } else {
        throw new Error('Missing export file')
      }
    },
    onSuccess: () => setConfirmation(undefined),
    onError: () => toast.error(t('Unable to download export.')),
  })
  const mutation = useMutation({
    mutationFn: submit,
    onSuccess: async (job, input) => {
      if (job.confirm_existing) {
        setConfirmation({ job, input })
        return
      }
      void queryClient.invalidateQueries({
        queryKey: ['customer-export-jobs'],
      })
      if (
        job.status === 'succeeded' &&
        (job.artifact?.files.length ?? 0) <= 1 &&
        (!job.artifact?.files.length || !hasExportCoverageFile(job))
      ) {
        try {
          const download = await requestExportDownload(job.job_id)
          if (download.emptyResult) {
            toast.info(t('No records to export.'))
            return
          }
          if (download.files.length === 1) {
            downloadExportFile(download.files[0].url)
            return
          }
        } catch {
          toast.error(t('Unable to download export.'))
        }
      }
      queryClient.setQueryData(['customer-export-job', job.job_id], job)
      setFocusedJobId(job.job_id)
    },
    onError: (error: Error) =>
      toast.error(error.message || t('Unable to submit export.')),
  })
  return {
    ...mutation,
    focusedJobId,
    confirmation,
    dismissConfirmation: () => setConfirmation(undefined),
    downloadExisting: existingDownload.mutate,
    downloadingExisting: existingDownload.isPending,
    clearFocus: () => setFocusedJobId(undefined),
  }
}
