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
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
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
import { ContractTemplateDrawer } from '@/features/customer-contracts/components/contract-template-drawer'
import {
  getContractTemplates,
  getContractTemplate,
} from '@/features/customer-contracts/template-api'
import type {
  ContractTemplateListItem,
  ContractTemplateSnapshot,
} from '@/features/customer-contracts/template-types'
import { templateRuleToDraft } from '@/features/customer-contracts/template-utils'
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

function buildDraft(contract: ContractEntityAdminView | null): ContractDraft {
  if (!contract) return { name: '', enabled: false, rules: [] }
  return {
    name: contract.name,
    enabled: contract.enabled,
    rules: contract.rules.map((rule) => ({
      ...rule,
      channel_id: rule.channel_id ?? 0,
    })),
  }
}

export function UserContractDrawer(props: UserContractDrawerProps) {
  const { t } = useTranslation()
  const [contracts, setContracts] = useState<ContractEntityAdminView[]>([])
  const [selectedId, setSelectedId] = useState<number | null>(null)
  const [creating, setCreating] = useState(false)
  const [channels, setChannels] = useState<
    CustomerContractChannelGroupOption[]
  >([])
  const [options, setOptions] = useState<CustomerContractGroupOption[]>([])
  const [draft, setDraft] = useState<ContractDraft>({
    name: '',
    enabled: false,
    rules: [],
  })
  const [reason, setReason] = useState('')
  const [addGroup, setAddGroup] = useState('')
  const [addModel, setAddModel] = useState('')
  const [addChannelIds, setAddChannelIds] = useState<string[]>([])
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
  const [templates, setTemplates] = useState<ContractTemplateListItem[]>([])
  const [selectedTemplate, setSelectedTemplate] = useState<{
    id: number
    version: number
    name: string
  } | null>(null)
  const [templateConflict, setTemplateConflict] =
    useState<ContractTemplateSnapshot | null>(null)
  const [saveAsOpen, setSaveAsOpen] = useState(false)
  const [saveAsDirtyWarn, setSaveAsDirtyWarn] = useState(false)
  const templateRequest = useRef(0)
  const pendingActionRef = useRef<(() => void) | null>(null)

  const selectedContract =
    contracts.find((contract) => contract.id === selectedId) ?? null

  useEffect(() => {
    if (!props.open) return
    let cancelled = false
    const loadTemplates = async () => {
      const items: ContractTemplateListItem[] = []
      for (let page = 1; ; page++) {
        const response = await getContractTemplates({ enabled: true, p: page, page_size: 100 })
        if (cancelled || !response.success || !response.data) return response
        items.push(...response.data.items)
        if (!response.data.items.length || items.length >= response.data.total) {
          return { ...response, data: { ...response.data, items } }
        }
      }
    }
    setLoading(true)
    void Promise.all([
      getUserContracts(props.user.id),
      getCustomerContractChannels(props.user.id),
      getCustomerContractOptions(props.user.id),
      loadTemplates(),
    ])
      .then(
        ([
          contractResponse,
          channelResponse,
          optionsResponse,
          templatesResponse,
        ]) => {
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
          const enabledTemplates =
            templatesResponse.success && templatesResponse.data
              ? templatesResponse.data.items || []
              : []
          if (!templatesResponse.success) {
            toast.error(t('Failed to load contract templates'))
          }
          setTemplates(enabledTemplates)
          const contractList = contractResponse.data.contracts || []
          const channelGroups = channelResponse.data || []
          const initial =
            contractList.find((contract) => contract.id === props.contractId) ??
            contractList[0] ??
            null
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
          setAddChannelIds([])
          setAddDiscount('1')
          setDirty(false)
          setSelectedTemplate(null)
          setTemplateConflict(null)
        }
      )
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
      templateRequest.current++
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
    templateRequest.current++
    setSelectedTemplate(null)
    setTemplateConflict(null)
    setSelectedId(contract.id)
    setCreating(false)
    setDraft(buildDraft(contract))
    setReason('')
    setRuleSearch('')
    setDirty(false)
  }

  const startCreate = () => {
    templateRequest.current++
    setSelectedId(null)
    setCreating(true)
    setDraft({ name: '', enabled: false, rules: [] })
    setReason('')
    setRuleSearch('')
    setDirty(false)
    setSelectedTemplate(null)
    setTemplateConflict(null)
  }

  // Applying a template fills the whole draft from server facts. A draft
  // with edits is only replaced after an explicit discard confirmation.
  const applyTemplate = (templateId: number) => {
    requestDiscard(() => {
      const request = ++templateRequest.current
      void getContractTemplate(templateId)
        .then((response) => {
          if (request !== templateRequest.current) return
          if (!response.success || !response.data) {
            toast.error(response.message || t('Loading failed'))
            return
          }
          const source = response.data
          setDraft({
            name: source.name,
            enabled: true,
            rules: source.rules.map((rule) =>
              templateRuleToDraft(rule, channels, options)
            ),
          })
          setSelectedTemplate({
            id: source.id,
            version: source.version,
            name: source.name,
          })
          setTemplateConflict(null)
          setDirty(true)
        })
        .catch(() => {
          if (request !== templateRequest.current) return
          toast.error(t('Loading failed'))
        })
    })
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
      ? channelOptionsForRule(channels, { route_group: addGroup, model })
      : []
    setAddChannelIds(candidates.length === 1 ? [String(candidates[0].id)] : [])
  }

  // One add commits one rule per selected channel. All of them share the
  // entered discount, so the same-model single-discount invariant holds by
  // construction; channels already bound to this model are rejected
  // explicitly instead of being skipped silently.
  const addRule = () => {
    if (!addGroup || !addModel) return
    const channelIds = addChannelIds.map(Number)
    if (channelIds.length === 0) {
      toast.error(t('Select a channel for this model'))
      return
    }
    const normalizedDiscount = normalizeContractDiscount(addDiscount)
    if (normalizedDiscount === null) {
      toast.error(t('Invalid contract discount'))
      return
    }
    const sameModelRules = draft.rules.filter(
      (rule) => rule.model.toLowerCase() === addModel.toLowerCase()
    )
    if (sameModelRules.some((rule) => rule.model !== addModel)) {
      toast.error(
        t('Model names that differ only by letter case cannot coexist')
      )
      return
    }
    const boundChannels = channelOptionsForRule(channels, {
      route_group: addGroup,
      model: addModel,
    }).filter(
      (channel) =>
        channelIds.includes(channel.id) &&
        sameModelRules.some((rule) => rule.channel_id === channel.id)
    )
    if (boundChannels.length > 0) {
      toast.error(
        t('This model already binds channels: {{channels}}', {
          channels: boundChannels.map((channel) => channel.name).join(', '),
        })
      )
      return
    }
    if (sameModelRules.some((rule) => rule.discount !== normalizedDiscount)) {
      toast.error(
        t(
          'All channels of one model must share the same contract discount in this save'
        )
      )
      return
    }
    const channelGroup = channels.find((group) => group.group === addGroup)
    const groupOption = options.find((option) => option.group === addGroup)
    const newRules = channelIds.map((channelId) => ({
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
    }))
    setDraft((current) => ({
      ...current,
      rules: [...current.rules, ...newRules],
    }))
    setAddModel('')
    setAddChannelIds([])
    setDirty(true)
  }

  // Template changed after apply: the two explicit resolutions from the
  // plan — keep the customized rules after confirming the latest version,
  // or re-apply the latest template rules. Closing the dialog changes
  // nothing and the draft stays intact.
  const confirmTemplateVersion = () => {
    if (!templateConflict) return
    const latestVersion = templateConflict.version
    setSelectedTemplate((current) =>
      current ? { ...current, version: latestVersion } : current
    )
    setTemplateConflict(null)
    toast.success(
      t('Latest template version confirmed. Review and save again.')
    )
  }

  const reapplyTemplateRules = () => {
    if (!templateConflict) return
    const source = templateConflict
    setDraft((current) => ({
      name: current.name,
      enabled: current.enabled,
      rules: source.rules.map((rule) =>
        templateRuleToDraft(rule, channels, options)
      ),
    }))
    setSelectedTemplate({
      id: source.id,
      version: source.version,
      name: source.name,
    })
    setTemplateConflict(null)
    setDirty(true)
  }

  // Save-as-template always copies the saved server version of this
  // contract, never unsaved draft edits.
  const startSaveAsTemplate = () => {
    if (!selectedContract) return
    if (dirty) {
      setSaveAsDirtyWarn(true)
      return
    }
    setSaveAsOpen(true)
  }

  const saveAsInitial = selectedContract
    ? { name: selectedContract.name, rules: buildDraft(selectedContract).rules }
    : null

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
      draft.rules.some(
        (rule) => !rule.model || !rule.route_group || !rule.discount
      )
    ) {
      toast.error(
        t('Every contract rule must include a model, route group, and discount')
      )
      return
    }
    if (
      draft.rules.some((rule) => parseContractDiscount(rule.discount) === null)
    ) {
      toast.error(t('Invalid contract discount'))
      return
    }
    if (draft.rules.some((rule) => rule.channel_id <= 0)) {
      toast.error(t('Select a channel for every contract rule'))
      return
    }
    // One save commits the complete ruleset: every rule of one model must
    // carry the same discount, so same-model channels cannot drift apart.
    const discountByModel = new Map<string, string>()
    for (const rule of draft.rules) {
      const normalized = parseContractDiscount(rule.discount)
      const key = rule.model.toLowerCase()
      const existing = discountByModel.get(key)
      if (existing === undefined) {
        discountByModel.set(key, String(normalized))
        continue
      }
      if (existing !== String(normalized)) {
        toast.error(
          t(
            'Model {{model}} must keep one identical discount across all of its channels in this save',
            { model: rule.model }
          )
        )
        return
      }
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
          ...(selectedTemplate
            ? {
                source_template_id: selectedTemplate.id,
                source_template_version: selectedTemplate.version,
              }
            : {}),
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
        creating &&
        selectedTemplate
      ) {
        // Template changed after apply: keep the whole draft, surface the
        // latest template and let the administrator confirm explicitly.
        try {
          const latest = await getContractTemplate(selectedTemplate.id)
          if (latest.success && latest.data) {
            setTemplateConflict(latest.data)
          } else {
            toast.error(latest.message || t('Loading failed'))
          }
        } catch {
          toast.error(t('Loading failed'))
        }
        return
      }
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
            requestDiscard(() => { templateRequest.current++; props.onOpenChange(false) })
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
                  const selected = !creating && contract.id === selectedId
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
                            'Bound keys use the selected contract models and channels. Native channel priority, weight and group pricing still apply.'
                          )
                        : t(
                            'This contract is disabled. Bound keys use their own group routing and pricing.'
                          )}
                    </AlertDescription>
                  </Alert>

                  {creating && (
                    <Field>
                      <FieldLabel htmlFor='contract-template-source'>
                        {t('Create from template')}
                      </FieldLabel>
                      <Select
                        items={[
                          { value: 'blank', label: t('Blank contract') },
                          ...templates.map((template) => ({
                            value: String(template.id),
                            label: template.name,
                          })),
                        ]}
                        value={
                          selectedTemplate
                            ? String(selectedTemplate.id)
                            : 'blank'
                        }
                        onValueChange={(value) => {
                          if (!value) return
                          if (value === 'blank') {
                            if (selectedTemplate) {
                              requestDiscard(() => {
                                templateRequest.current++
                                setDraft({
                                  name: '',
                                  enabled: false,
                                  rules: [],
                                })
                                setReason('')
                                setSelectedTemplate(null)
                                setTemplateConflict(null)
                                setDirty(false)
                              })
                            }
                            return
                          }
                          const templateId = Number(value)
                          if (templateId !== selectedTemplate?.id) {
                            applyTemplate(templateId)
                          }
                        }}
                      >
                        <SelectTrigger
                          id='contract-template-source'
                          className='w-full'
                        >
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectGroup>
                            <SelectItem value='blank'>
                              {t('Blank contract')}
                            </SelectItem>
                            {templates.map((template) => (
                              <SelectItem
                                key={template.id}
                                value={String(template.id)}
                              >
                                {template.name}
                              </SelectItem>
                            ))}
                          </SelectGroup>
                        </SelectContent>
                      </Select>
                      {selectedTemplate && (
                        <p className='text-muted-foreground text-xs'>
                          {t(
                            'Applied template version {{version}}. Rules stay editable and failures must be fixed before saving.',
                            { version: selectedTemplate.version }
                          )}
                        </p>
                      )}
                    </Field>
                  )}

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
                    group={addGroup}
                    model={addModel}
                    channelIds={addChannelIds}
                    discount={addDiscount}
                    onGroupChange={setAddGroup}
                    onModelChange={handleAddModelChange}
                    onChannelsChange={setAddChannelIds}
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
                                'The contract is enabled but has no available models. Bound keys cannot submit model requests.'
                              )
                            : t(
                                'Add a model rule to define which models receive this contract discount.'
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
                onClick={() => requestDiscard(() => { templateRequest.current++; props.onOpenChange(false) })}
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
              {!creating && selectedContract && (
                <Button
                  type='button'
                  variant='outline'
                  disabled={saving}
                  onClick={startSaveAsTemplate}
                >
                  {t('Save as template')}
                </Button>
              )}
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
          'Disabling this contract immediately restores bound keys to their own group routing and pricing. This may allow channels outside the contract.'
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

      <ConfirmDialog
        open={templateConflict !== null}
        onOpenChange={(open) => {
          if (!open) setTemplateConflict(null)
        }}
        title={t('Contract template changed')}
        desc={t(
          'The template was changed after you applied it and now sits at version {{version}}. Your draft is kept. Confirm the latest version to keep your customized rules, or re-apply the latest template rules.',
          { version: templateConflict?.version ?? 0 }
        )}
        cancelBtnText={t('Decide later')}
        confirmText={t('Re-apply template rules')}
        handleConfirm={reapplyTemplateRules}
      >
        <Button variant='outline' onClick={confirmTemplateVersion}>
          {t('Keep my rules and confirm the latest version')}
        </Button>
      </ConfirmDialog>

      <ConfirmDialog
        open={saveAsDirtyWarn}
        onOpenChange={setSaveAsDirtyWarn}
        title={t('Save this contract as a template?')}
        desc={t(
          'Unsaved edits in this drawer are not included: saving as a template always copies the saved contract version.'
        )}
        confirmText={t('Continue and copy saved version')}
        handleConfirm={() => {
          setSaveAsDirtyWarn(false)
          setSaveAsOpen(true)
        }}
      />

      {saveAsOpen && saveAsInitial && (
        <ContractTemplateDrawer
          open
          onOpenChange={setSaveAsOpen}
          templateId={null}
          initial={saveAsInitial}
        />
      )}
    </>
  )
}
