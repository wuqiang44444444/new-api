import { isAxiosError } from 'axios'
import { AlertTriangle, Plus } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from '@/components/ui/empty'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'

import {
  createUserContract,
  getContractEntityAudits,
  getCustomerContractChannels,
  getCustomerContractOptions,
  getUserContracts,
  updateContractEntity,
} from '../api'
import type {
  ContractEntityAdminView,
  ContractRuleDraft,
  CustomerContractAudit,
  CustomerContractChannelGroupOption,
  CustomerContractGroupOption,
  User,
} from '../types'
import { CustomerContractAddRule } from './user-contract-add-rule'
import { CustomerContractAuditHistory } from './user-contract-audit'
import { CustomerContractRuleList } from './user-contract-rule-list'
import {
  channelOptionsForRule,
  normalizeContractDiscount,
  parseContractDiscount,
} from './user-contract-utils'

interface ContractDraft {
  name: string
  enabled: boolean
  rules: ContractRuleDraft[]
}

interface UserContractDrawerProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  user: Pick<User, 'id' | 'username'>
  onSuccess: () => void
  contractId?: number
}

function buildDraft(
  contract: ContractEntityAdminView | null
): ContractDraft {
  if (!contract) return { name: '', enabled: false, rules: [] }
  return {
    name: contract.name,
    enabled: contract.enabled,
    rules: contract.rules.map((rule) =>
      ({ ...rule, channel_id: rule.channel_id ?? 0 })
    ),
  }
}

export function UserContractDrawer(props: UserContractDrawerProps) {
  const { t } = useTranslation()
  const [contracts, setContracts] = useState<ContractEntityAdminView[]>([])
  const [selectedId, setSelectedId] = useState<number | null>(null)
  const [creating, setCreating] = useState(false)
  const [channels, setChannels] = useState<CustomerContractChannelGroupOption[]>(
    []
  )
  const [options, setOptions] = useState<CustomerContractGroupOption[]>([])
  const [draft, setDraft] = useState<ContractDraft>({
    name: '',
    enabled: false,
    rules: [],
  })
  const [reason, setReason] = useState('')
  const [addGroup, setAddGroup] = useState('')
  const [addModel, setAddModel] = useState('')
  const [addChannel, setAddChannel] = useState('')
  const [addDiscount, setAddDiscount] = useState('1')
  const [ruleSearch, setRuleSearch] = useState('')
  const [audits, setAudits] = useState<CustomerContractAudit[]>([])
  const [auditPage, setAuditPage] = useState(1)
  const [auditTotal, setAuditTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [dirty, setDirty] = useState(false)
  const [disableConfirmOpen, setDisableConfirmOpen] = useState(false)
  const [discardConfirmOpen, setDiscardConfirmOpen] = useState(false)
  const pendingActionRef = useRef<(() => void) | null>(null)

  const selectedContract =
    contracts.find((contract) => contract.id === selectedId) ?? null

  useEffect(() => {
    if (!props.open) return
    let cancelled = false
    setLoading(true)
    void Promise.all([
      getUserContracts(props.user.id),
      getCustomerContractChannels(props.user.id),
      getCustomerContractOptions(props.user.id),
    ])
      .then(([contractResponse, channelResponse, optionsResponse]) => {
        if (cancelled) return
        if (!contractResponse.success || !contractResponse.data) {
          throw new Error(contractResponse.message || t('Loading failed'))
        }
        if (!channelResponse.success || !channelResponse.data) {
          throw new Error(channelResponse.message || t('Loading failed'))
        }
        if (!optionsResponse.success || !optionsResponse.data) {
          throw new Error(optionsResponse.message || t('Loading failed'))
        }
        const contractList = contractResponse.data.contracts || []
        const channelGroups = channelResponse.data || []
        const initial =
          contractList.find(
            (contract) => contract.id === props.contractId
          ) ?? contractList[0] ?? null
        setContracts(contractList)
        setChannels(channelGroups)
        setOptions(optionsResponse.data || [])
        setSelectedId(initial?.id ?? null)
        setCreating(!initial)
        setDraft(buildDraft(initial))
        setReason('')
        setRuleSearch('')
        setAddGroup(channelGroups[0]?.group || '')
        setAddModel('')
        setAddChannel('')
        setAddDiscount('1')
        setDirty(false)
      })
      .catch((error: unknown) => {
        if (!cancelled) {
          toast.error(
            error instanceof Error ? error.message : t('Loading failed')
          )
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [props.open, props.user.id, props.contractId, t])

  useEffect(() => {
    if (!props.open || creating || !selectedId) {
      setAudits([])
      setAuditTotal(0)
      setAuditPage(1)
      return
    }
    let cancelled = false
    void getContractEntityAudits(selectedId, 1)
      .then((response) => {
        if (!cancelled && response.success) {
          setAudits(response.data?.items || [])
          setAuditTotal(response.data?.total || 0)
          setAuditPage(1)
        }
      })
      .catch(() => {
        if (!cancelled) toast.error(t('Loading failed'))
      })
    return () => {
      cancelled = true
    }
  }, [props.open, creating, selectedId, t])

  const requestDiscard = (action: () => void) => {
    if (!dirty) {
      action()
      return
    }
    pendingActionRef.current = action
    setDiscardConfirmOpen(true)
  }

  const runPendingAction = () => {
    const action = pendingActionRef.current
    pendingActionRef.current = null
    setDiscardConfirmOpen(false)
    setDirty(false)
    action?.()
  }

  const selectContract = (contract: ContractEntityAdminView) => {
    setSelectedId(contract.id)
    setCreating(false)
    setDraft(buildDraft(contract))
    setReason('')
    setRuleSearch('')
    setDirty(false)
  }

  const startCreate = () => {
    setSelectedId(null)
    setCreating(true)
    setDraft({ name: '', enabled: false, rules: [] })
    setReason('')
    setRuleSearch('')
    setDirty(false)
  }

  const updateRule = (index: number, patch: Partial<ContractRuleDraft>) => {
    setDraft((current) => ({
      ...current,
      rules: current.rules.map((rule, ruleIndex) =>
        ruleIndex === index ? { ...rule, ...patch } : rule
      ),
    }))
    setDirty(true)
  }

  const handleAddModelChange = (model: string) => {
    setAddModel(model)
    // With exactly one qualifying channel there is nothing to decide.
    const candidates = model
      ? channelOptionsForRule(
          channels,
          { route_group: addGroup, model }
        )
      : []
    setAddChannel(candidates.length === 1 ? String(candidates[0].id) : '')
  }

  const addRule = () => {
    if (!addGroup || !addModel) return
    const channelId = Number(addChannel)
    if (!channelId) {
      toast.error(t('Select a channel for this model'))
      return
    }
    const normalizedDiscount = normalizeContractDiscount(addDiscount)
    if (normalizedDiscount === null) {
      toast.error(t('Invalid contract discount'))
      return
    }
    if (
      draft.rules.some(
        (rule) => rule.model.toLowerCase() === addModel.toLowerCase()
      )
    ) {
      toast.error(
        t('Model names that differ only by letter case cannot coexist')
      )
      return
    }
    const channelGroup = channels.find((group) => group.group === addGroup)
    const groupOption = options.find((option) => option.group === addGroup)
    setDraft((current) => ({
      ...current,
      rules: [
        ...current.rules,
        {
          model: addModel,
          channel_id: channelId,
          route_group: addGroup,
          discount: normalizedDiscount,
          available: true,
          native_group_ratio: channelGroup?.native_group_ratio || '1',
          effective_multiplier: channelGroup?.native_group_ratio || '1',
          special_group_ratio: channelGroup?.special_group_ratio || false,
          price: groupOption?.prices?.[addModel] || {
            price_type: 'model_ratio' as const,
          },
        },
      ],
    }))
    setAddModel('')
    setAddChannel('')
    setDirty(true)
  }

  const reloadAfterConflict = async (contractId: number) => {
    const [latest, latestAudits] = await Promise.all([
      getUserContracts(props.user.id),
      getContractEntityAudits(contractId, 1),
    ])
    if (latest.success && latest.data) {
      const contractList = latest.data.contracts || []
      setContracts(contractList)
      const current =
        contractList.find((contract) => contract.id === contractId) ??
        contractList[0] ??
        null
      setSelectedId(current?.id ?? null)
      setCreating(!current)
      setDraft(buildDraft(current))
      setReason('')
      setDirty(false)
    }
    if (latestAudits.success && latestAudits.data) {
      setAudits(latestAudits.data.items)
      setAuditTotal(latestAudits.data.total)
      setAuditPage(1)
    }
  }

  const save = async (nextEnabled = draft.enabled) => {
    let auditContractId: number | null = null
    const name = draft.name.trim()
    if (!name) {
      toast.error(t('Contract name is required'))
      return
    }
    if (!reason.trim()) {
      toast.error(t('Change reason is required'))
      return
    }
    if (
      draft.rules.some((rule) => !rule.model || !rule.route_group || !rule.discount)
    ) {
      toast.error(
        t('Every contract rule must include a model, route group, and discount')
      )
      return
    }
    if (draft.rules.some((rule) => parseContractDiscount(rule.discount) === null)) {
      toast.error(t('Invalid contract discount'))
      return
    }
    if (draft.rules.some((rule) => rule.channel_id <= 0)) {
      toast.error(t('Select a channel for every contract rule'))
      return
    }
    const payloadRules = draft.rules.map((rule) => ({
      model: rule.model,
      channel_id: rule.channel_id,
      route_group: rule.route_group,
      discount: rule.discount,
    }))
    setSaving(true)
    try {
      if (creating || !selectedContract) {
        const response = await createUserContract(props.user.id, {
          name,
          enabled: nextEnabled,
          reason: reason.trim(),
          rules: payloadRules,
        })
        if (!response.success || !response.data) {
          throw new Error(response.message || t('Save failed'))
        }
        const createdId = response.data.contracts[0]?.id ?? null
        const listResponse = await getUserContracts(props.user.id)
        if (listResponse.success && listResponse.data) {
          const contractList = listResponse.data.contracts || []
          setContracts(contractList)
          const created =
            contractList.find((contract) => contract.id === createdId) ?? null
          setSelectedId(created?.id ?? null)
          setCreating(!created)
          setDraft(buildDraft(created))
        }
        setReason('')
        setDirty(false)
        toast.success(t('Contract created'))
      } else {
        const response = await updateContractEntity(selectedContract.id, {
          expected_version: selectedContract.version,
          name,
          enabled: nextEnabled,
          reason: reason.trim(),
          rules: payloadRules,
        })
        if (!response.success || !response.data) {
          throw new Error(response.message || t('Save failed'))
        }
        const updated = response.data.contracts[0] ?? null
        if (updated) {
          setContracts((current) =>
            current.map((contract) =>
              contract.id === updated.id ? updated : contract
            )
          )
          setDraft(buildDraft(updated))
        }
        setReason('')
        setDirty(false)
        auditContractId = selectedContract.id
        toast.success(t('Customer contract saved'))
      }
      props.onSuccess()
    } catch (error: unknown) {
      if (
        isAxiosError(error) &&
        error.response?.status === 409 &&
        selectedContract
      ) {
        try {
          await reloadAfterConflict(selectedContract.id)
          toast.error(
            t(
              'Contract changed by another administrator. Latest version loaded.'
            )
          )
          return
        } catch {
          toast.error(t('Contract changed. Reload the editor and try again.'))
          return
        }
      }
      toast.error(error instanceof Error ? error.message : t('Save failed'))
    } finally {
      setSaving(false)
      setDisableConfirmOpen(false)
    }
    if (auditContractId !== null) {
      try {
        const latestAudits = await getContractEntityAudits(auditContractId, 1)
        if (!latestAudits.success || !latestAudits.data) {
          throw new Error('audit refresh failed')
        }
        setAudits(latestAudits.data.items)
        setAuditTotal(latestAudits.data.total)
        setAuditPage(1)
      } catch {
        toast.error(t('Loading failed'))
      }
    }
  }

  let saveLabel = creating ? t('Create contract') : t('Save contract')
  if (saving) {
    saveLabel = t('Saving...')
  } else if (creating && draft.enabled) {
    saveLabel = t('Save and enable contract')
  }

  return (
    <>
      <Sheet
        open={props.open}
        onOpenChange={(nextOpen) => {
          if (!nextOpen) {
            requestDiscard(() => props.onOpenChange(false))
            return
          }
          props.onOpenChange(nextOpen)
        }}
      >
        <SheetContent className='w-[96vw] sm:max-w-[1080px]'>
          <SheetHeader className='border-b'>
            <SheetTitle>{t('Manage model contracts')}</SheetTitle>
            <SheetDescription>
              {props.user.username} · {t('User ID')} {props.user.id} ·{' '}
              {t('{{count}} contracts', { count: contracts.length })}
            </SheetDescription>
          </SheetHeader>

          <div className='flex flex-col gap-2 border-b px-4 py-3'>
            <div className='flex items-center justify-between'>
              <span className='text-muted-foreground text-xs font-medium'>
                {t('Contracts')}
              </span>
              <Button
                type='button'
                variant='outline'
                size='sm'
                disabled={loading || saving}
                onClick={() => requestDiscard(startCreate)}
              >
                <Plus data-icon='inline-start' />
                {t('New contract')}
              </Button>
            </div>
            {contracts.length === 0 ? (
              <div className='text-muted-foreground text-sm'>
                {loading ? t('Loading...') : t('No contract models')}
              </div>
            ) : (
              <div className='grid gap-2 sm:grid-cols-2 lg:grid-cols-3'>
                {contracts.map((contract) => {
                  const selected =
                    !creating && contract.id === selectedId
                  return (
                    <button
                      key={contract.id}
                      type='button'
                      onClick={() => {
                        if (selected) return
                        requestDiscard(() => selectContract(contract))
                      }}
                      className={cn(
                        'flex flex-col items-start gap-1 rounded-md border p-2 text-left transition-colors',
                        selected
                          ? 'border-primary bg-primary/5'
                          : 'hover:bg-muted/40'
                      )}
                    >
                      <span className='w-full truncate text-sm font-medium'>
                        {contract.name}
                      </span>
                      <span className='text-muted-foreground text-xs'>
                        v{contract.version} · {contract.rules.length}{' '}
                        {t('Rules')}
                      </span>
                      <Badge
                        variant={contract.enabled ? 'secondary' : 'outline'}
                      >
                        {contract.enabled ? t('Enabled') : t('Disabled')}
                      </Badge>
                    </button>
                  )
                })}
              </div>
            )}
          </div>

          <Tabs defaultValue='configuration' className='min-h-0 flex-1 px-4'>
            <TabsList>
              <TabsTrigger value='configuration'>
                {t('Configuration')}
              </TabsTrigger>
              <TabsTrigger value='audit'>{t('Audit history')}</TabsTrigger>
            </TabsList>

            <TabsContent
              value='configuration'
              className='min-h-0 overflow-y-auto pb-4'
            >
              {loading ? (
                <div className='text-muted-foreground py-12 text-center'>
                  {t('Loading...')}
                </div>
              ) : (
                <FieldGroup>
                  <Alert>
                    <AlertTriangle />
                    <AlertTitle>
                      {draft.enabled
                        ? t('Contract mode is active')
                        : t('Contract mode is inactive')}
                    </AlertTitle>
                    <AlertDescription>
                      {draft.enabled
                        ? t(
                            'API keys bound to this contract can only call the models listed below.'
                          )
                        : t(
                            'API keys bound to this contract currently follow native model permissions and pricing.'
                          )}
                    </AlertDescription>
                  </Alert>

                  <Field>
                    <FieldLabel htmlFor='contract-entity-name'>
                      {t('Contract name')}
                    </FieldLabel>
                    <Input
                      id='contract-entity-name'
                      value={draft.name}
                      maxLength={128}
                      onChange={(event) => {
                        const name = event.target.value
                        setDraft((current) => ({ ...current, name }))
                        setDirty(true)
                      }}
                      placeholder={t('Enter a name')}
                    />
                  </Field>

                  <CustomerContractAddRule
                    channelGroups={channels}
                    existingModels={draft.rules.map((rule) =>
                      rule.model.toLowerCase()
                    )}
                    group={addGroup}
                    model={addModel}
                    channelId={addChannel}
                    discount={addDiscount}
                    onGroupChange={setAddGroup}
                    onModelChange={handleAddModelChange}
                    onChannelChange={setAddChannel}
                    onDiscountChange={setAddDiscount}
                    onAdd={addRule}
                  />

                  {draft.rules.length === 0 ? (
                    <Empty className='border'>
                      <EmptyHeader>
                        <EmptyTitle>{t('No contract models')}</EmptyTitle>
                        <EmptyDescription>
                          {draft.enabled
                            ? t(
                                'The contract is active, so all model calls are currently denied.'
                              )
                            : t(
                                'Add a model rule to define which models this contract can call.'
                              )}
                        </EmptyDescription>
                      </EmptyHeader>
                    </Empty>
                  ) : (
                    <CustomerContractRuleList
                      rules={draft.rules}
                      channelGroups={channels}
                      search={ruleSearch}
                      onSearchChange={setRuleSearch}
                      onUpdate={updateRule}
                      onRemove={(index) => {
                        setDraft((current) => ({
                          ...current,
                          rules: current.rules.filter(
                            (_, ruleIndex) => ruleIndex !== index
                          ),
                        }))
                        setDirty(true)
                      }}
                    />
                  )}

                  <Field>
                    <FieldLabel htmlFor='contract-change-reason'>
                      {t('Change reason')}
                    </FieldLabel>
                    <Textarea
                      id='contract-change-reason'
                      value={reason}
                      onChange={(event) => {
                        setReason(event.target.value)
                        setDirty(true)
                      }}
                      maxLength={500}
                      placeholder={t('Required for audit history')}
                    />
                  </Field>
                </FieldGroup>
              )}
            </TabsContent>

            <TabsContent value='audit' className='min-h-0 overflow-y-auto pb-4'>
              {creating || !selectedId ? (
                <Empty className='border'>
                  <EmptyHeader>
                    <EmptyTitle>
                      {t('Save the contract to see its history')}
                    </EmptyTitle>
                  </EmptyHeader>
                </Empty>
              ) : (
                <CustomerContractAuditHistory
                  contractId={selectedId}
                  audits={audits}
                  page={auditPage}
                  total={auditTotal}
                  setAudits={setAudits}
                  setPage={setAuditPage}
                />
              )}
            </TabsContent>
          </Tabs>

          <SheetFooter className='border-t sm:flex-row sm:justify-between'>
            <div className='flex gap-2'>
              <Button
                type='button'
                variant='outline'
                disabled={saving}
                onClick={() => requestDiscard(() => props.onOpenChange(false))}
              >
                {t('Cancel')}
              </Button>
              <Button
                type='button'
                variant={draft.enabled ? 'destructive' : 'outline'}
                disabled={loading || saving}
                onClick={() => {
                  if (draft.enabled) {
                    setDisableConfirmOpen(true)
                  } else {
                    setDraft((current) => ({ ...current, enabled: true }))
                    setDirty(true)
                  }
                }}
              >
                {draft.enabled
                  ? t('Disable contract mode')
                  : t('Enable contract mode')}
              </Button>
            </div>
            <Button
              type='button'
              disabled={!dirty || saving || loading}
              onClick={() => save()}
            >
              {saveLabel}
            </Button>
          </SheetFooter>
        </SheetContent>
      </Sheet>

      <ConfirmDialog
        open={disableConfirmOpen}
        onOpenChange={setDisableConfirmOpen}
        title={t('Disable contract mode?')}
        desc={t(
          'API keys bound to this contract will immediately return to native model permissions and pricing.'
        )}
        destructive
        confirmText={t('Disable and save')}
        isLoading={saving}
        handleConfirm={() => save(false)}
      />

      <ConfirmDialog
        open={discardConfirmOpen}
        onOpenChange={setDiscardConfirmOpen}
        title={t('Discard unsaved contract changes?')}
        desc={t(
          'Your unsaved model, channel, discount, and status changes will be lost.'
        )}
        destructive
        confirmText={t('Discard changes')}
        handleConfirm={runPendingAction}
      />
    </>
  )
}
