import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  downloadExportFile,
  requestExportDownload,
  type CustomerExportJobView,
} from './export-api'

export function useBillingExportJob(
  submit: () => Promise<CustomerExportJobView>
) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [focusedJobId, setFocusedJobId] = useState<string>()
  const mutation = useMutation({
    mutationFn: submit,
    onSuccess: async (job) => {
      void queryClient.invalidateQueries({ queryKey: ['customer-export-jobs'] })
      if (
        job.status === 'succeeded' &&
        (job.artifact?.files.length ?? 0) <= 1
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
    clearFocus: () => setFocusedJobId(undefined),
  }
}
