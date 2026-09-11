import { useQuery, useQueryClient } from '@tanstack/react-query'
import { isAxiosError } from 'axios'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from '@/components/ui/empty'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'

import {
  getCustomerContractMigrationPreview,
  migrateCustomerContract,
} from '../api'
import type {
  CustomerContractMigrationPreview,
  CustomerContractMigrationRulePreview,
} from '../types'
import {
  CustomerContractBulkMigration,
  type ContractMigrationTarget,
} from './customer-contract-bulk-migration'

type ChannelSelection = Record<string, number>

type UserMigrationState = {
  selections: ChannelSelection
  contractName: string
  reason: string
  submitting: boolean
}

function formatRatioUnits(units: number): string {
  const value = Number(units) / 1e8
  if (!Number.isFinite(value) || value <= 0) return '—'
  return value.toFixed(8).replace(/\.?0+$/, '')
}

function initialSelections(
  rules: CustomerContractMigrationRulePreview[]
): ChannelSelection {
  const selections: ChannelSelection = {}
  for (const rule of rules) {
    if (rule.resolved_channel_id > 0) {
      selections[rule.public_model] = rule.resolved_channel_id
    }
  }
  return selections
}

export function CustomerContractMigrationDialog(props: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [bulkRunning, setBulkRunning] = useState(false)
  const [states, setStates] = useState<Record<number, UserMigrationState>>({})
  const previewQuery = useQuery({
    queryKey: ['customer-contracts-migration-preview'],
    queryFn: async () => {
      const response = await getCustomerContractMigrationPreview()
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Loading failed'))
      }
      return response.data
    },
    enabled: props.open,
    refetchOnWindowFocus: !bulkRunning,
  })

  const previews = previewQuery.data ?? []

  const getUserState = (
    user: CustomerContractMigrationPreview
  ): UserMigrationState => {
    const state = states[user.user_id] ?? {
      selections: initialSelections(user.rules),
      contractName: '',
      reason: '',
      submitting: false,
    }
    const selections = { ...initialSelections(user.rules) }
    for (const rule of user.rules) {
      const selected = state.selections[rule.public_model]
      if (rule.channel_ids.includes(selected)) {
        selections[rule.public_model] = selected
      }
    }
    return { ...state, selections }
  }

  const updateUserState = (
    userId: number,
    patch: Partial<UserMigrationState>
  ) => {
    setStates((current) => {
      const user = previews.find((candidate) => candidate.user_id === userId)
      if (!user) return current
      const base = current[userId] ?? {
        selections: initialSelections(user.rules),
        contractName: '',
        reason: '',
        submitting: false,
      }
      return { ...current, [userId]: { ...base, ...patch } }
    })
  }

  const invalidate = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['customer-contracts'] }),
      queryClient.invalidateQueries({
        queryKey: ['customer-contracts-migration-preview'],
      }),
    ])
  }

  const busy =
    bulkRunning || Object.values(states).some((state) => state.submitting)
  const targets: ContractMigrationTarget[] = []
  let unresolvedCount = 0
  for (const user of previews) {
    if (user.already_migrated) continue
    const state = getUserState(user)
    if (
      !user.rules.length ||
      user.rules.some((rule) => !state.selections[rule.public_model])
    ) {
      unresolvedCount++
      continue
    }
    targets.push({
      username: user.username,
      payload: {
        user_id: user.user_id,
        contract_name: state.contractName.trim(),
        channel_overrides: state.selections,
      },
    })
  }

  const submit = async (user: CustomerContractMigrationPreview) => {
    const state = getUserState(user)
    if (busy) return
    const missing = user.rules.filter(
      (rule) => !state.selections[rule.public_model]
    )
    if (missing.length > 0) {
      toast.error(
        t('Select a channel for every contract rule that needs a decision.')
      )
      return
    }
    if (!state.reason.trim()) {
      toast.error(t('Change reason is required'))
      return
    }
    updateUserState(user.user_id, { submitting: true })
    try {
      const response = await migrateCustomerContract({
        user_id: user.user_id,
        contract_name: state.contractName.trim(),
        reason: state.reason.trim(),
        channel_overrides: state.selections,
      })
      if (!response.success) {
        throw new Error(response.message || t('Save failed'))
      }
      toast.success(t('Contract created'))
      await invalidate()
    } catch (error: unknown) {
      if (isAxiosError(error) && error.response?.status === 409) {
        toast.error(
          t('This user has already been migrated or has no rules to migrate.')
        )
        return
      }
      toast.error(error instanceof Error ? error.message : t('Save failed'))
    } finally {
      updateUserState(user.user_id, { submitting: false })
    }
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => !busy && props.onOpenChange(open)}
    >
      <DialogContent
        showCloseButton={!busy}
        className='max-h-[85vh] overflow-y-auto sm:max-w-[720px]'
      >
        <DialogHeader>
          <DialogTitle>{t('Legacy contract migrations')}</DialogTitle>
          <DialogDescription>
            {t(
              'Convert legacy user-level contracts into contract entities; ambiguous channel mappings require an explicit decision.'
            )}
          </DialogDescription>
        </DialogHeader>
        <CustomerContractBulkMigration
          targets={targets}
          unresolvedCount={unresolvedCount}
          disabled={busy || !previewQuery.isSuccess || previewQuery.isFetching}
          onRunningChange={setBulkRunning}
          onComplete={invalidate}
        />
        <Button
          type='button'
          variant='outline'
          disabled={busy || previewQuery.isFetching}
          onClick={() => previewQuery.refetch()}
        >
          {t('Refresh')}
        </Button>
        {previewQuery.isLoading && (
          <div className='text-muted-foreground py-8 text-center text-sm'>
            {t('Loading...')}
          </div>
        )}
        {!previewQuery.isLoading && previewQuery.isError && (
          <div className='text-destructive py-8 text-center text-sm'>
            {previewQuery.error instanceof Error
              ? previewQuery.error.message
              : t('Loading failed')}
          </div>
        )}
        {!previewQuery.isLoading &&
          !previewQuery.isError &&
          previews.length === 0 && (
            <Empty className='border'>
              <EmptyHeader>
                <EmptyTitle>{t('No legacy contracts to migrate')}</EmptyTitle>
                <EmptyDescription>
                  {t(
                    'Every legacy user-level contract has already been migrated.'
                  )}
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          )}
        {!previewQuery.isLoading &&
          !previewQuery.isError &&
          previews.length > 0 && (
            <div className='flex flex-col gap-4'>
              {previews.map((user) => {
                const state = getUserState(user)
                return (
                  <div key={user.user_id} className='rounded-lg border p-3'>
                    <div className='flex flex-wrap items-center justify-between gap-2'>
                      <div>
                        <div className='text-sm font-medium'>
                          {user.username}
                        </div>
                        <div className='text-muted-foreground text-xs'>
                          {t('User ID')} {user.user_id} · {t('Bound keys')}:{' '}
                          {user.bound_token_count}
                        </div>
                      </div>
                      <div className='flex items-center gap-2'>
                        {user.contract_enabled && (
                          <Badge variant='secondary'>{t('Enabled')}</Badge>
                        )}
                        {user.already_migrated && (
                          <Badge variant='outline'>{t('Migrated')}</Badge>
                        )}
                      </div>
                    </div>
                    {user.already_migrated ? (
                      <div className='text-muted-foreground mt-2 text-sm'>
                        {t('This user has already been migrated.')}
                      </div>
                    ) : (
                      <>
                        <div className='mt-3 flex flex-col gap-2'>
                          {user.rules.map((rule) => {
                            const selected =
                              state.selections[rule.public_model] || 0
                            return (
                              <div
                                key={rule.public_model}
                                className='grid gap-2 rounded-md border p-2 lg:grid-cols-[minmax(180px,1fr)_130px_110px_minmax(200px,1fr)] lg:items-end'
                              >
                                <div className='min-w-0'>
                                  <div className='truncate font-mono text-sm'>
                                    {rule.public_model}
                                  </div>
                                  {rule.needs_decision && (
                                    <div className='text-muted-foreground text-xs'>
                                      {t('Channel must be decided')}
                                    </div>
                                  )}
                                </div>
                                <div className='text-muted-foreground text-sm'>
                                  {rule.route_group}
                                </div>
                                <div className='font-mono text-sm'>
                                  {formatRatioUnits(rule.ratio_units)}
                                </div>
                                <Select
                                  disabled={busy}
                                  items={rule.channel_ids.map((id) => ({
                                    value: String(id),
                                    label: `#${id}`,
                                  }))}
                                  value={String(selected || '')}
                                  onValueChange={(value) => {
                                    if (!value) return
                                    updateUserState(user.user_id, {
                                      selections: {
                                        ...state.selections,
                                        [rule.public_model]: Number(value),
                                      },
                                    })
                                  }}
                                >
                                  <SelectTrigger
                                    aria-label={`${rule.public_model} ${t('Channel')}`}
                                  >
                                    <SelectValue>
                                      {selected
                                        ? `#${selected}`
                                        : t('Select channel')}
                                    </SelectValue>
                                  </SelectTrigger>
                                  <SelectContent alignItemWithTrigger={false}>
                                    <SelectGroup>
                                      {rule.channel_ids.map((id) => (
                                        <SelectItem key={id} value={String(id)}>
                                          #{id}
                                        </SelectItem>
                                      ))}
                                    </SelectGroup>
                                  </SelectContent>
                                </Select>
                              </div>
                            )
                          })}
                        </div>
                        <div className='mt-3 grid gap-2 md:grid-cols-2'>
                          <Field>
                            <FieldLabel
                              htmlFor={`migration-name-${user.user_id}`}
                            >
                              {t('Contract name')}
                            </FieldLabel>
                            <Input
                              disabled={busy}
                              id={`migration-name-${user.user_id}`}
                              value={state.contractName}
                              maxLength={128}
                              placeholder={t('Legacy contract')}
                              onChange={(event) =>
                                updateUserState(user.user_id, {
                                  contractName: event.target.value,
                                })
                              }
                            />
                          </Field>
                          <Field>
                            <FieldLabel
                              htmlFor={`migration-reason-${user.user_id}`}
                            >
                              {t('Change reason')}
                            </FieldLabel>
                            <Textarea
                              disabled={busy}
                              id={`migration-reason-${user.user_id}`}
                              value={state.reason}
                              maxLength={500}
                              onChange={(event) =>
                                updateUserState(user.user_id, {
                                  reason: event.target.value,
                                })
                              }
                            />
                          </Field>
                        </div>
                        <DialogFooter>
                          <Button
                            type='button'
                            disabled={
                              busy ||
                              user.rules.some(
                                (rule) =>
                                  rule.needs_decision &&
                                  !state.selections[rule.public_model]
                              )
                            }
                            onClick={() => submit(user)}
                          >
                            {state.submitting ? t('Saving...') : t('Migrate')}
                          </Button>
                        </DialogFooter>
                      </>
                    )}
                  </div>
                )
              })}
            </div>
          )}
      </DialogContent>
    </Dialog>
  )
}
