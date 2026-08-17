import { isAxiosError } from 'axios'
import { AlertTriangle } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from '@/components/ui/empty'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
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

import {
  getCustomerContract,
  getCustomerContractAudits,
  getCustomerContractOptions,
  updateCustomerContract,
} from '../api'
import type {
  CustomerContract,
  CustomerContractAudit,
  CustomerContractGroupOption,
  CustomerContractRule,
  User,
} from '../types'
import { CustomerContractAddRule } from './user-contract-add-rule'
import { CustomerContractAuditHistory } from './user-contract-audit'
import { CustomerContractRuleList } from './user-contract-rule-list'
import {
  normalizeContractDiscount,
  parseContractDiscount,
} from './user-contract-utils'

interface UserContractDrawerProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  user: User
  onSuccess: () => void
}

export function UserContractDrawer({
  open,
  onOpenChange,
  user,
  onSuccess,
}: UserContractDrawerProps) {
  const { t } = useTranslation()
  const [contract, setContract] = useState<CustomerContract | null>(null)
  const [options, setOptions] = useState<CustomerContractGroupOption[]>([])
  const [audits, setAudits] = useState<CustomerContractAudit[]>([])
  const [auditPage, setAuditPage] = useState(1)
  const [auditTotal, setAuditTotal] = useState(0)
  const [enabled, setEnabled] = useState(false)
  const [rules, setRules] = useState<CustomerContractRule[]>([])
  const [reason, setReason] = useState('')
  const [addGroup, setAddGroup] = useState('')
  const [addModels, setAddModels] = useState<string[]>([])
  const [addDiscount, setAddDiscount] = useState('1')
  const [ruleSearch, setRuleSearch] = useState('')
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [dirty, setDirty] = useState(false)
  const [disableConfirmOpen, setDisableConfirmOpen] = useState(false)
  const [closeConfirmOpen, setCloseConfirmOpen] = useState(false)

  useEffect(() => {
    if (!open) return
    let cancelled = false
    setLoading(true)
    void Promise.all([
      getCustomerContract(user.id),
      getCustomerContractOptions(user.id),
    ])
      .then(([contractResponse, optionsResponse]) => {
        if (cancelled) return
        if (!contractResponse.success || !contractResponse.data) {
          throw new Error(contractResponse.message || t('Loading failed'))
        }
        if (!optionsResponse.success || !optionsResponse.data) {
          throw new Error(optionsResponse.message || t('Loading failed'))
        }
        setContract(contractResponse.data)
        setEnabled(contractResponse.data.contract_mode)
        setRules(contractResponse.data.rules)
        setOptions(optionsResponse.data || [])
        setReason('')
        setDirty(false)
        setAddGroup(optionsResponse.data?.[0]?.group || '')
        setAddModels([])
        setAddDiscount('1')
        setRuleSearch('')
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
    setAuditPage(1)
    void getCustomerContractAudits(user.id, 1)
      .then((response) => {
        if (!cancelled && response.success) {
          setAudits(response.data?.items || [])
          setAuditTotal(response.data?.total || 0)
        }
      })
      .catch(() => {
        if (!cancelled) toast.error(t('Loading failed'))
      })
    return () => {
      cancelled = true
    }
  }, [open, t, user.id])

  let saveLabel = t('Save contract')
  if (saving) {
    saveLabel = t('Saving...')
  } else if ((contract?.contract_version || 0) === 0 && rules.length > 0) {
    saveLabel = t('Save and enable contract')
  }

  const requestClose = (nextOpen: boolean) => {
    if (!nextOpen && dirty) {
      setCloseConfirmOpen(true)
      return
    }
    onOpenChange(nextOpen)
  }

  const updateRule = (index: number, patch: Partial<CustomerContractRule>) => {
    setRules((current) =>
      current.map((rule, ruleIndex) =>
        ruleIndex === index ? { ...rule, ...patch } : rule
      )
    )
    setDirty(true)
  }

  const addRule = () => {
    if (!addGroup || addModels.length === 0) return
    const normalizedDiscount = normalizeContractDiscount(addDiscount)
    if (normalizedDiscount === null) {
      toast.error(t('Invalid contract discount'))
      return
    }
    if (
      new Set(addModels.map((model) => model.toLowerCase())).size !==
      addModels.length
    ) {
      toast.error(
        t('Model names that differ only by letter case cannot coexist')
      )
      return
    }
    const groupOption = options.find((option) => option.group === addGroup)
    setRules((current) => [
      ...current,
      ...addModels.map((model) => ({
        model,
        route_group: addGroup,
        discount: normalizedDiscount,
        available: true,
        native_group_ratio: groupOption?.native_group_ratio || '1',
        effective_multiplier: groupOption?.native_group_ratio || '1',
        special_group_ratio: groupOption?.special_group_ratio || false,
        price: groupOption?.prices?.[model] || {
          price_type: 'model_ratio' as const,
        },
      })),
    ])
    if ((contract?.contract_version || 0) === 0) setEnabled(true)
    setAddModels([])
    setDirty(true)
  }

  const save = async (nextEnabled = enabled) => {
    if (!contract) return
    if (!reason.trim()) {
      toast.error(t('Change reason is required'))
      return
    }
    if (
      rules.some((rule) => !rule.model || !rule.route_group || !rule.discount)
    ) {
      toast.error(
        t('Every contract rule must include a model, route group, and discount')
      )
      return
    }
    if (rules.some((rule) => parseContractDiscount(rule.discount) === null)) {
      toast.error(t('Invalid contract discount'))
      return
    }
    setSaving(true)
    try {
      const response = await updateCustomerContract(user.id, {
        expected_version: contract.contract_version,
        enabled: nextEnabled,
        reason: reason.trim(),
        rules: rules.map((rule) => ({
          model: rule.model,
          route_group: rule.route_group,
          discount: rule.discount,
        })),
      })
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Save failed'))
      }
      setContract(response.data)
      setEnabled(response.data.contract_mode)
      setRules(response.data.rules)
      setReason('')
      setDirty(false)
      try {
        const auditResponse = await getCustomerContractAudits(user.id, 1)
        setAudits(auditResponse.data?.items || [])
        setAuditTotal(auditResponse.data?.total || 0)
        setAuditPage(1)
      } catch {
        toast.error(t('Loading failed'))
      }
      toast.success(t('Customer contract saved'))
      onSuccess()
    } catch (error: unknown) {
      if (isAxiosError(error) && error.response?.status === 409) {
        try {
          const [latest, latestAudits] = await Promise.all([
            getCustomerContract(user.id),
            getCustomerContractAudits(user.id, 1),
          ])
          if (latest.success && latest.data) {
            setContract(latest.data)
            setEnabled(latest.data.contract_mode)
            setRules(latest.data.rules)
            setReason('')
            setDirty(false)
          }
          if (latestAudits.success && latestAudits.data) {
            setAudits(latestAudits.data.items)
            setAuditTotal(latestAudits.data.total)
            setAuditPage(1)
          }
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
  }

  return (
    <>
      <Sheet open={open} onOpenChange={requestClose}>
        <SheetContent className='w-[96vw] sm:max-w-[1080px]'>
          <SheetHeader className='border-b'>
            <SheetTitle>{t('Manage model contract')}</SheetTitle>
            <SheetDescription>
              {user.username} · {t('User ID')} {user.id} ·{' '}
              {enabled
                ? t('Contract mode is active')
                : t('Contract mode is inactive')}{' '}
              · {rules.length} {t('Rules')} · {t('Contract version')}{' '}
              {contract?.contract_version || 0}
            </SheetDescription>
          </SheetHeader>

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
                      {enabled
                        ? t('Contract mode is active')
                        : t('Contract mode is inactive')}
                    </AlertTitle>
                    <AlertDescription>
                      {enabled
                        ? t(
                            'All existing and future API keys can only call the models listed below.'
                          )
                        : t(
                            'This user currently follows native NEWAPI permissions and pricing.'
                          )}
                    </AlertDescription>
                  </Alert>

                  <CustomerContractAddRule
                    options={options}
                    rules={rules}
                    group={addGroup}
                    setGroup={setAddGroup}
                    models={addModels}
                    setModels={setAddModels}
                    discount={addDiscount}
                    setDiscount={setAddDiscount}
                    onAdd={addRule}
                  />

                  {rules.length === 0 ? (
                    <Empty className='border'>
                      <EmptyHeader>
                        <EmptyTitle>{t('No contract models')}</EmptyTitle>
                        <EmptyDescription>
                          {enabled
                            ? t(
                                'The contract is active, so all model calls are currently denied.'
                              )
                            : t(
                                'Add a model rule to create and activate this customer contract.'
                              )}
                        </EmptyDescription>
                      </EmptyHeader>
                    </Empty>
                  ) : (
                    <CustomerContractRuleList
                      rules={rules}
                      options={options}
                      search={ruleSearch}
                      onSearchChange={setRuleSearch}
                      onUpdate={updateRule}
                      onRemove={(index) => {
                        setRules((current) =>
                          current.filter((_, ruleIndex) => ruleIndex !== index)
                        )
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
              <CustomerContractAuditHistory
                userId={user.id}
                audits={audits}
                page={auditPage}
                total={auditTotal}
                setAudits={setAudits}
                setPage={setAuditPage}
              />
            </TabsContent>
          </Tabs>

          <SheetFooter className='border-t sm:flex-row sm:justify-between'>
            <div className='flex gap-2'>
              <Button
                type='button'
                variant='outline'
                disabled={saving}
                onClick={() => requestClose(false)}
              >
                {t('Cancel')}
              </Button>
              <Button
                type='button'
                variant={enabled ? 'destructive' : 'outline'}
                disabled={loading || saving || !contract}
                onClick={() => {
                  if (enabled) {
                    setDisableConfirmOpen(true)
                  } else {
                    setEnabled(true)
                    setDirty(true)
                  }
                }}
              >
                {enabled
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
        desc={
          contract?.disable_warning ||
          t(
            'All existing API keys will immediately return to native model permissions and pricing.'
          )
        }
        destructive
        confirmText={t('Disable and save')}
        isLoading={saving}
        handleConfirm={() => save(false)}
      />

      <ConfirmDialog
        open={closeConfirmOpen}
        onOpenChange={setCloseConfirmOpen}
        title={t('Discard unsaved contract changes?')}
        desc={t(
          'Your unsaved model, route group, discount, and status changes will be lost.'
        )}
        destructive
        confirmText={t('Discard changes')}
        handleConfirm={() => {
          setCloseConfirmOpen(false)
          setDirty(false)
          onOpenChange(false)
        }}
      />
    </>
  )
}
