/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { isAxiosError } from 'axios'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { DateTimePicker } from '@/components/datetime-picker'
import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'
import { Separator } from '@/components/ui/separator'
import { formatTimestampToDate } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { ROLE } from '@/lib/roles'
import { createServerError } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import {
  getCurrentEvidenceCleanupTask,
  listSystemTasks,
  startEvidenceCleanupTask,
} from '../api'
import type { EvidenceCleanupTask, SystemTaskResponse } from '../types'
import { EvidenceCleanupHealth } from './evidence-cleanup-health'
import { EvidenceCleanupSummary } from './evidence-cleanup-summary'

export function EvidenceCleanup() {
  const userID = useAuthStore((state) =>
    state.auth.user?.role === ROLE.SUPER_ADMIN ? state.auth.user.id : null
  )
  if (!userID) return null
  return (
    <>
      <Separator />
      <RootEvidenceCleanup userID={userID} />
    </>
  )
}

function RootEvidenceCleanup(props: { userID: number }) {
  const { t } = useTranslation()
  const titleID = useId()
  const queryClient = useQueryClient()
  const queryKey = ['system-settings', 'evidence-cleanup', props.userID]
  const [purgeDate, setPurgeDate] = useState<Date | undefined>(
    () => new Date(Date.now() - 7 * 86400000)
  )
  const [confirmOpen, setConfirmOpen] = useState(false)
  const query = useQuery({
    queryKey,
    queryFn: async () => {
      const [current, history] = await Promise.all([
        getCurrentEvidenceCleanupTask(),
        listSystemTasks(1, {
          type: 'task_request_evidence_retention',
          scope: 'history',
        }),
      ])
      if (!current.success || !history.success) {
        throw createServerError(
          !current.success ? current : history,
          t('Failed to load request evidence cleanup')
        )
      }
      return (
        current.data ??
        (history.data?.[0] as EvidenceCleanupTask | undefined) ??
        null
      )
    },
    refetchInterval: (state) =>
      state.state.data?.status === 'pending' ||
      state.state.data?.status === 'running'
        ? 1000
        : 30000,
    retry: false,
    meta: { errorToast: false },
  })
  const task = query.data
  const healthQuery = useQuery({
    queryKey: ['system-settings', 'evidence-cleanup-health', props.userID],
    queryFn: async () => {
      const res = await listSystemTasks(1, {
        type: 'task_request_evidence_monitor',
        scope: 'history',
      })
      if (!res.success) {
        throw createServerError(res, t('Evidence cleanup health check failed.'))
      }
      const latest = res.data?.[0]
      if (latest?.status === 'failed' && !latest.result) {
        return { alerts: ['health_check_failed'] }
      }
      return latest?.result ?? null
    },
    retry: false,
    refetchInterval: 30000,
    meta: { errorToast: false },
  })
  const active = task?.status === 'pending' || task?.status === 'running'
  const mutation = useMutation({
    mutationFn: async (timestamp: number) => {
      try {
        const res = await startEvidenceCleanupTask(timestamp)
        if (!res.success || !res.data) {
          throw createServerError(res, t('Failed to clean request evidence'))
        }
        return { task: res.data, busy: false }
      } catch (error) {
        if (
          isAxiosError<SystemTaskResponse<EvidenceCleanupTask>>(error) &&
          error.response?.status === 409 &&
          error.response.data.data
        ) {
          return { task: error.response.data.data, busy: true }
        }
        throw error
      }
    },
    onSuccess: (result) => {
      queryClient.setQueryData(queryKey, result.task)
      setConfirmOpen(false)
      if (result.busy) {
        toast.info(t('An evidence cleanup task is already running.'))
      } else {
        toast.success(t('Evidence cleanup task started.'))
      }
    },
    onError: (error) =>
      handleServerError(error, t('Failed to clean request evidence')),
  })
  const timestamp = purgeDate ? Math.floor(purgeDate.getTime() / 1000) : 0
  const valid =
    Number.isSafeInteger(timestamp) &&
    timestamp > 0 &&
    timestamp <= Math.floor(Date.now() / 1000)
  const formattedDate =
    purgeDate && valid
      ? formatTimestampToDate(purgeDate.getTime(), 'milliseconds')
      : ''
  const zone = Intl.DateTimeFormat().resolvedOptions().timeZone
  const busy = active || mutation.isPending

  return (
    <div
      role='group'
      aria-labelledby={titleID}
      className='flex min-w-0 flex-col gap-4'
    >
      <div>
        <h4 id={titleID} className='font-medium'>
          {t('Clean request evidence')}
        </h4>
        <p className='text-muted-foreground mt-1 text-sm'>
          {t(
            'Remove evidence bodies and event details created before the selected timestamp. Closed indexes may remain for up to seven days.'
          )}
        </p>
      </div>
      <DateTimePicker value={purgeDate} onChange={setPurgeDate} />
      <p className='text-muted-foreground text-sm'>
        {t(' (time zone: {{zone}}).', { zone })}
      </p>
      {!valid && (
        <p role='alert' className='text-destructive text-sm'>
          {t('Select a valid timestamp not later than now.')}
        </p>
      )}
      <div className='flex flex-wrap gap-3'>
        <Button
          variant='outline'
          type='button'
          disabled={busy}
          onClick={() => setPurgeDate(new Date(Date.now() - 86400000))}
        >
          {t('24 hours ago')}
        </Button>
        <Button
          variant='outline'
          type='button'
          disabled={busy}
          onClick={() => setPurgeDate(new Date(Date.now() - 7 * 86400000))}
        >
          {t('7 days ago')}
        </Button>
        <Button
          type='button'
          variant='destructive'
          disabled={busy || !valid || query.isPending || query.isError}
          onClick={() => setConfirmOpen(true)}
        >
          {busy ? t('Cleaning...') : t('Clean evidence')}
        </Button>
        <Button
          type='button'
          variant='outline'
          disabled={query.isFetching}
          onClick={() => void query.refetch()}
        >
          {t('Refresh')}
        </Button>
      </div>
      {query.isError && (
        <ErrorState
          title={t('Failed to load request evidence cleanup')}
          onRetry={() => void query.refetch()}
        />
      )}
      {healthQuery.isError && (
        <ErrorState
          title={t('Evidence cleanup health check failed.')}
          onRetry={() => void healthQuery.refetch()}
        />
      )}
      <EvidenceCleanupHealth health={healthQuery.data} />
      {task && (
        <div
          role='status'
          aria-live='polite'
          className='space-y-3 rounded-md border p-3'
        >
          <p className='font-medium'>
            {t('Evidence cleanup progress')} · {t(task.status)}
          </p>
          {active && (
            <Progress
              value={null}
              aria-label={t('Evidence cleanup progress')}
            />
          )}
          <EvidenceCleanupSummary
            stats={task.result ?? task.state}
            payload={task.payload}
          />
          {task.status === 'failed' && (
            <p className='text-destructive text-sm'>
              {t('Failed to clean request evidence')}
            </p>
          )}
        </div>
      )}
      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={(open) => {
          if (!mutation.isPending) setConfirmOpen(open)
        }}
        title={t('Confirm evidence cleanup')}
        desc={
          <>
            {t(
              'This will permanently remove evidence bodies and event details created before {{date}}',
              { date: formattedDate }
            )}
            {t(' (time zone: {{zone}}).', { zone })}{' '}
            {t(
              'This includes historical evidence of all types and cannot be undone. Closed indexes expire within seven days. Tasks and billing records are kept.'
            )}
          </>
        }
        destructive
        disabled={!valid || active}
        isLoading={mutation.isPending}
        confirmText={t('Delete evidence')}
        handleConfirm={() => {
          if (valid && !busy) mutation.mutate(timestamp)
        }}
      />
    </div>
  )
}
