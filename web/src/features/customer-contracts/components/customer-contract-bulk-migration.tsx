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
import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Field, FieldLabel } from '@/components/ui/field'
import { Progress } from '@/components/ui/progress'
import { Textarea } from '@/components/ui/textarea'

import { migrateCustomerContract } from '../api'
import type { CustomerContractMigrationPayload } from '../types'

export interface ContractMigrationTarget {
  username: string
  payload: Omit<CustomerContractMigrationPayload, 'reason'>
}

interface BulkMigrationProps {
  targets: ContractMigrationTarget[]
  unresolvedCount: number
  disabled: boolean
  onRunningChange: (running: boolean) => void
  onComplete: () => Promise<void>
}

export function CustomerContractBulkMigration(props: BulkMigrationProps) {
  const { t } = useTranslation()
  const [confirmation, setConfirmation] = useState<
    ContractMigrationTarget[] | null
  >(null)
  const [reason, setReason] = useState('')
  const [report, setReport] = useState<{
    total: number
    results: { userId: number; username: string; success: boolean }[]
  }>({ total: 0, results: [] })

  const migration = useMutation({
    retry: false,
    onMutate: () => props.onRunningChange(true),
    mutationFn: async (batch: {
      targets: ContractMigrationTarget[]
      reason: string
    }) => {
      setConfirmation(null)
      setReport({ total: batch.targets.length, results: [] })
      for (const target of batch.targets) {
        let success = false
        try {
          const response = await migrateCustomerContract({
            ...target.payload,
            reason: batch.reason,
          })
          success = response.success
        } catch {
          // A lost response may follow a committed migration. The refreshed preview
          // is authoritative; do not resend or claim the contract was rolled back.
        }
        setReport((current) => ({
          ...current,
          results: [
            ...current.results,
            {
              userId: target.payload.user_id,
              username: target.username,
              success,
            },
          ],
        }))
      }
    },
    onSettled: async () => {
      try {
        await props.onComplete()
      } finally {
        props.onRunningChange(false)
      }
    },
  })

  const succeeded = report.results.filter((result) => result.success).length
  const unconfirmed = report.results.filter((result) => !result.success)

  return (
    <div className='bg-muted/30 space-y-3 rounded-lg border p-3'>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <div className='space-y-1 text-sm'>
          <p className='font-medium'>
            {t('Migrate legacy contracts in one batch')}
          </p>
          <p className='text-muted-foreground'>
            {t('Ready: {{ready}} · Needs a channel decision: {{blocked}}', {
              ready: props.targets.length,
              blocked: props.unresolvedCount,
            })}
          </p>
        </div>
        <Button
          disabled={
            props.disabled || migration.isPending || props.targets.length === 0
          }
          onClick={() => {
            setReason(t('Bulk legacy contract migration'))
            setConfirmation(props.targets)
          }}
        >
          {t('One-click migration')}
        </Button>
      </div>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Already migrated contracts are skipped. Resolve missing channel choices below, then migrate the remaining contracts together.'
        )}
      </p>
      {report.total > 0 && (
        <div className='space-y-2' role='status' aria-live='polite'>
          <p className='text-sm'>
            {t(
              'Processed {{done}} / {{total}} · Migrated {{success}} · Unconfirmed {{unconfirmed}}',
              {
                done: report.results.length,
                total: report.total,
                success: succeeded,
                unconfirmed: unconfirmed.length,
              }
            )}
          </p>
          <Progress
            value={report.results.length}
            max={report.total}
            aria-label={t('Migration progress')}
          />
          {migration.isPending && (
            <p className='text-muted-foreground text-xs'>
              {t(
                'Migration is running. Keep this page open until it finishes.'
              )}
            </p>
          )}
          {unconfirmed.length > 0 && (
            <div className='space-y-2'>
              <p className='text-sm'>
                {t(
                  'Migration not confirmed. Refresh the preview before retrying.'
                )}
              </p>
              <div className='flex flex-wrap gap-2'>
                {unconfirmed.map((result) => (
                  <Badge key={result.userId} variant='outline'>
                    {result.username} · #{result.userId}
                  </Badge>
                ))}
              </div>
            </div>
          )}
        </div>
      )}
      <ConfirmDialog
        open={confirmation !== null}
        onOpenChange={(open) =>
          !open && !migration.isPending && setConfirmation(null)
        }
        title={t('One-click migration')}
        desc={t(
          'Migrate {{count}} customer contracts with the displayed channel choices. Existing unbound API keys will be bound to the migrated contracts; existing contract bindings are preserved. Confirm that the database backup and migration checks are complete.',
          { count: confirmation?.length ?? 0 }
        )}
        confirmText={t('Start migration')}
        disabled={props.disabled || !reason.trim()}
        isLoading={migration.isPending}
        handleConfirm={() => {
          if (
            confirmation?.length &&
            !migration.isPending &&
            !props.disabled &&
            reason.trim()
          ) {
            migration.mutate({ targets: confirmation, reason: reason.trim() })
          }
        }}
      >
        <Field>
          <FieldLabel htmlFor='bulk-migration-reason'>
            {t('Batch change reason')}
          </FieldLabel>
          <Textarea
            id='bulk-migration-reason'
            value={reason}
            maxLength={500}
            disabled={migration.isPending}
            onChange={(event) => setReason(event.target.value)}
          />
        </Field>
      </ConfirmDialog>
    </div>
  )
}
