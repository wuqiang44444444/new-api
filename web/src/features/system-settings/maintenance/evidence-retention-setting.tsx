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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { createServerError } from '@/lib/server-error-message'

import { getEvidenceRetentionPolicy } from '../api'
import { useUpdateOption } from '../hooks/use-update-option'
import type { EvidenceRetentionPolicy } from '../types'

export function EvidenceRetentionSetting(props: { userID: number }) {
  const { t } = useTranslation()
  const inputID = useId()
  const helpID = useId()
  const queryClient = useQueryClient()
  const queryKey = ['system-settings', 'evidence-retention', props.userID]
  const query = useQuery({
    queryKey,
    queryFn: async () => {
      const response = await getEvidenceRetentionPolicy()
      if (!response.success || !response.data) {
        throw createServerError(
          response,
          t('Failed to load evidence retention')
        )
      }
      return response.data
    },
    retry: false,
    meta: { errorToast: false },
  })
  const updateOption = useUpdateOption()
  const [draft, setDraft] = useState<string>()
  const [confirmOpen, setConfirmOpen] = useState(false)
  const policy = query.data
  if (!policy) {
    return query.isError ? (
      <ErrorState
        title={t('Failed to load evidence retention')}
        onRetry={() => void query.refetch()}
      />
    ) : (
      <p role='status'>{t('Loading evidence retention…')}</p>
    )
  }

  const value = draft ?? String(policy.days)
  const days = Number(value)
  const valid = Number.isInteger(days) && days >= 1 && days <= 7
  const canSave =
    !updateOption.isPending &&
    valid &&
    (days !== policy.days || policy.source !== 'database')
  const sourceLabels = {
    database: t('Database'),
    environment: t('Deployment environment'),
    default: t('Built-in default'),
  }
  const save = () => {
    if (!canSave) return
    updateOption.mutate(
      { key: 'task_request_evidence.retention_days', value: days },
      {
        onSuccess: () => {
          queryClient.setQueryData<EvidenceRetentionPolicy>(queryKey, {
            days,
            source: 'database',
          })
          void queryClient.invalidateQueries({ queryKey })
          setDraft(undefined)
          setConfirmOpen(false)
        },
      }
    )
  }

  return (
    <form
      className='flex min-w-0 flex-col gap-3'
      onSubmit={(event) => {
        event.preventDefault()
        if (!canSave) return
        if (days < policy.days) setConfirmOpen(true)
        else save()
      }}
    >
      <Label htmlFor={inputID}>{t('Request evidence retention (days)')}</Label>
      <p id={helpID} className='text-muted-foreground text-sm'>
        {t('Current retention is {{days}} days ({{source}}).', {
          days: policy.days,
          source: sourceLabels[policy.source],
        })}{' '}
        {t('Other instances apply changes on their next settings sync.')}
      </p>
      <div className='flex flex-wrap items-center gap-2'>
        <Input
          id={inputID}
          className='w-24'
          type='number'
          min={1}
          max={7}
          step={1}
          required
          aria-describedby={helpID}
          value={value}
          disabled={updateOption.isPending}
          onChange={(event) => setDraft(event.target.value)}
        />
        <Button type='submit' disabled={!canSave}>
          {t('Save retention')}
        </Button>
      </div>
      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t('Shorten request evidence retention?')}
        desc={t(
          'Existing evidence older than the new limit becomes unreadable as the setting takes effect and is removed by scheduled cleanup.'
        )}
        confirmText={t('Save retention')}
        destructive
        disabled={!canSave}
        isLoading={updateOption.isPending}
        handleConfirm={save}
      />
    </form>
  )
}
