import { Info } from 'lucide-react'
import { useEffect, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

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
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { CustomerContractAddRule } from '@/features/users/components/user-contract-add-rule'
import { CustomerContractRuleList } from '@/features/users/components/user-contract-rule-list'
import {
  buildContractBatchRules,
  channelOptionsForRule,
  parseContractDiscount,
} from '@/features/users/components/user-contract-utils'
import type {
  ContractRuleDraft,
  CustomerContractChannelGroupOption,
  CustomerContractGroupOption,
} from '@/features/users/types'
import { formatTimestamp } from '@/lib/format'

import {
  createContractTemplate,
  getContractTemplate,
  getContractTemplateAudits,
  getContractTemplateOptions,
  updateContractTemplate,
} from '../template-api'
import type {
  ContractTemplateAudit,
  ContractTemplateSnapshot,
} from '../template-types'
import { templateRuleToDraft } from '../template-utils'

interface TemplateDraft {
  name: string
  enabled: boolean
  rules: ContractRuleDraft[]
}

export interface ContractTemplateDrawerProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Editing an existing template when set; creating otherwise. */
  templateId?: number | null
  /** Prefill for "save as template" from a saved contract. */
  initial?: { name: string; rules: ContractRuleDraft[] } | null
  onSaved?: () => void
}

export function ContractTemplateDrawer(props: ContractTemplateDrawerProps) {
  const { t } = useTranslation()
  const [savedTemplateId, setSavedTemplateId] = useState<number | null>(
    props.templateId ?? null
  )
  const editing = savedTemplateId !== null
  const [channels, setChannels] = useState<
    CustomerContractChannelGroupOption[]
  >([])
  const [options, setOptions] = useState<CustomerContractGroupOption[]>([])
  const [draft, setDraft] = useState<TemplateDraft>({
    name: props.initial?.name ?? '',
    enabled: Boolean(props.initial) || !editing,
    rules: props.initial?.rules ?? [],
  })
  const [reason, setReason] = useState('')
  const [addGroup, setAddGroup] = useState('')
  const [addModels, setAddModels] = useState<string[]>([])
  const [addChannelIdsByModel, setAddChannelIdsByModel] = useState<
    Record<string, string[]>
  >({})
  const [addDiscount, setAddDiscount] = useState('1')
  const [ruleSearch, setRuleSearch] = useState('')
  const [audits, setAudits] = useState<ContractTemplateAudit[]>([])
  const [auditPage, setAuditPage] = useState(1)
  const [auditTotal, setAuditTotal] = useState(0)
  const [auditError, setAuditError] = useState(false)
  const [auditLoading, setAuditLoading] = useState(false)
  const [version, setVersion] = useState<number | null>(null)
  const [loading, setLoading] = useState(false)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [reloadToken, setReloadToken] = useState(0)
  const [saving, setSaving] = useState(false)
  const [dirty, setDirty] = useState(false)

  // Load options and, when editing, the template snapshot itself. Server
  // facts win over the prefill; a failed load surfaces an error instead of
  // pretending the template has no rules.
  const templateId = props.templateId ?? null
  useEffect(() => {
    if (!props.open) return
    let cancelled = false
    setLoading(true)
    setLoadError(null)
    const load = async () => {
      try {
        const [optionsResponse, snapshotResponse] = await Promise.all([
          getContractTemplateOptions(),
          templateId ? getContractTemplate(templateId) : Promise.resolve(null),
        ])
        if (cancelled) return
        if (!optionsResponse.success || !optionsResponse.data) {
          throw new Error(optionsResponse.message || t('Loading failed'))
        }
        let source: ContractTemplateSnapshot | null = null
        if (snapshotResponse) {
          if (!snapshotResponse.success || !snapshotResponse.data) {
            throw new Error(snapshotResponse.message || t('Loading failed'))
          }
          source = snapshotResponse.data
        }
        const channelGroups = optionsResponse.data.channels || []
        const groupOptions = optionsResponse.data.options || []
        setChannels(channelGroups)
        setOptions(groupOptions)
        setAddGroup(channelGroups[0]?.group || '')
        if (source) {
          setDraft({
            name: source.name,
            enabled: source.enabled,
            rules: source.rules.map((rule) =>
              templateRuleToDraft(rule, channelGroups, groupOptions)
            ),
          })
          setVersion(source.version)
        } else if (props.initial) {
          setDraft((current) => ({
            ...current,
            rules: current.rules.map((rule) =>
              templateRuleToDraft(
                {
                  public_model: rule.model,
                  channel_id: rule.channel_id,
                  route_group: rule.route_group,
                  ratio_units: Math.round(Number(rule.discount) * 100_000_000),
                  available: rule.available,
                },
                channelGroups,
                groupOptions
              )
            ),
          }))
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    load().catch((error: unknown) => {
      if (cancelled) return
      const message =
        error instanceof Error ? error.message : t('Loading failed')
      setLoadError(message)
      toast.error(message)
    })
    return () => {
      cancelled = true
    }
    // Runs once per open/template switch; parents mount this drawer fresh.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [props.open, templateId, reloadToken])

  const updateRule = (index: number, patch: Partial<ContractRuleDraft>) => {
    setDraft((current) => ({
      ...current,
      rules: current.rules.map((rule, ruleIndex) =>
        ruleIndex === index ? { ...rule, ...patch } : rule
      ),
    }))
    setDirty(true)
  }

  const handleAddModelsChange = (values: string[]) => {
    setAddModels(values)
    // A fresh pick initializes from the current candidates (single candidate
    // auto-picked); removing a model drops its channels, so re-adding it
    // re-initializes. Still-selected models keep their explicit picks.
    setAddChannelIdsByModel((current) => {
      const next: Record<string, string[]> = {}
      for (const model of values) {
        next[model] =
          model in current
            ? current[model]
            : (() => {
                const candidates = channelOptionsForRule(channels, {
                  route_group: addGroup,
                  model,
                })
                return candidates.length === 1 ? [String(candidates[0].id)] : []
              })()
      }
      return next
    })
  }

  const handleModelChannelsChange = (model: string, values: string[]) => {
    setAddChannelIdsByModel((current) => ({ ...current, [model]: values }))
  }

  // One add validates the entire pending batch against itself and the draft,
  // then appends all of its rules at once. All rules share the entered
  // discount, so the same-model single-discount invariant holds by
  // construction; any conflict rejects the whole batch and keeps the inputs.
  const addRules = () => {
    if (!addGroup || addModels.length === 0) return
    const result = buildContractBatchRules({
      channelGroups: channels,
      groupOptions: options,
      draftRules: draft.rules,
      routeGroup: addGroup,
      models: addModels,
      channelIdsByModel: addChannelIdsByModel,
      discount: addDiscount,
    })
    if (!result.ok) {
      toast.error(t(result.error.key, result.error.params))
      return
    }
    if (result.rules.length === 0) return
    setDraft((current) => ({
      ...current,
      rules: [...current.rules, ...result.rules],
    }))
    setAddModels([])
    setAddChannelIdsByModel({})
    setDirty(true)
  }

  const save = async () => {
    const name = draft.name.trim()
    if (!name) {
      toast.error(t('Template name is required'))
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
    const payloadRules = draft.rules.map((rule) => ({
      model: rule.model,
      channel_id: rule.channel_id,
      route_group: rule.route_group,
      discount: rule.discount,
    }))
    setSaving(true)
    try {
      const response = editing
        ? await updateContractTemplate(savedTemplateId as number, {
            expected_version: version ?? 0,
            enabled: draft.enabled,
            name,
            reason: reason.trim(),
            rules: payloadRules,
          })
        : await createContractTemplate({
            enabled: draft.enabled,
            name,
            reason: reason.trim(),
            rules: payloadRules,
          })
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Save failed'))
      }
      const saved = response.data
      setDraft({
        name: saved.name,
        enabled: saved.enabled,
        rules: saved.rules.map((rule) =>
          templateRuleToDraft(rule, channels, options)
        ),
      })
      setSavedTemplateId(saved.id)
      setVersion(saved.version)
      setReason('')
      setDirty(false)
      toast.success(t('Contract template saved'))
      props.onSaved?.()
    } catch (error: unknown) {
      toast.error(error instanceof Error ? error.message : t('Save failed'))
    } finally {
      setSaving(false)
    }
  }

  const loadAudits = (nextPage: number) => {
    if (!savedTemplateId) return
    setAuditLoading(true)
    setAuditError(false)
    return getContractTemplateAudits(savedTemplateId, nextPage)
      .then((response) => {
        if (response.success && response.data) {
          setAudits(response.data.items)
          setAuditTotal(response.data.total)
          setAuditPage(nextPage)
        } else {
          setAuditError(true)
        }
      })
      .catch(() => {
        setAuditError(true)
      })
      .finally(() => setAuditLoading(false))
  }

  let configurationBody: ReactNode
  if (loading) {
    configurationBody = (
      <div className='text-muted-foreground py-12 text-center'>
        {t('Loading...')}
      </div>
    )
  } else if (loadError) {
    configurationBody = (
      <Alert>
        <Info />
        <AlertTitle>{t('Loading failed')}</AlertTitle>
        <AlertDescription>
          <p>{loadError}</p>
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={() => setReloadToken((token) => token + 1)}
          >
            {t('Retry')}
          </Button>
        </AlertDescription>
      </Alert>
    )
  } else {
    configurationBody = (
      <FieldGroup>
        <Alert>
          <Info />
          <AlertTitle>{t('No customer price context')}</AlertTitle>
          <AlertDescription>
            {t(
              'Template price references use the plain native group ratio. Target customers settle with their own group ratios and the contract discount.'
            )}
          </AlertDescription>
        </Alert>

        <Field>
          <FieldLabel htmlFor='contract-template-name'>
            {t('Template name')}
          </FieldLabel>
          <Input
            id='contract-template-name'
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

        <Field className='flex-row items-center justify-between rounded-lg border p-3'>
          <div>
            <FieldLabel htmlFor='contract-template-enabled'>
              {t('Enable template')}
            </FieldLabel>
            <p className='text-muted-foreground text-xs'>
              {t('Only enabled templates can create customer contracts.')}
            </p>
          </div>
          <Switch
            id='contract-template-enabled'
            checked={draft.enabled}
            onCheckedChange={(checked) => {
              setDraft((current) => ({ ...current, enabled: checked }))
              setDirty(true)
            }}
          />
        </Field>

        <CustomerContractAddRule
          channelGroups={channels}
          group={addGroup}
          models={addModels}
          channelIdsByModel={addChannelIdsByModel}
          discount={addDiscount}
          onGroupChange={setAddGroup}
          onModelsChange={handleAddModelsChange}
          onModelChannelsChange={handleModelChannelsChange}
          onDiscountChange={setAddDiscount}
          onAdd={addRules}
        />

        {draft.rules.length === 0 ? (
          <Empty className='border'>
            <EmptyHeader>
              <EmptyTitle>{t('No contract models')}</EmptyTitle>
              <EmptyDescription>
                {t(
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
          <FieldLabel htmlFor='contract-template-reason'>
            {t('Change reason')}
          </FieldLabel>
          <Textarea
            id='contract-template-reason'
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
    )
  }

  return (
    <Sheet open={props.open} onOpenChange={props.onOpenChange}>
      <SheetContent className='w-[96vw] sm:max-w-[1080px]'>
        <SheetHeader className='border-b'>
          <SheetTitle>
            {editing ? t('Edit contract template') : t('New contract template')}
          </SheetTitle>
          <SheetDescription>
            {t(
              'Templates are shared by all administrators and only prefill new customer contracts.'
            )}
          </SheetDescription>
        </SheetHeader>

        <Tabs
          onValueChange={(value) => {
            if (value === 'audit') loadAudits(1)
          }}
          defaultValue='configuration'
          className='min-h-0 flex-1 px-4'
        >
          <TabsList>
            <TabsTrigger value='configuration'>
              {t('Configuration')}
            </TabsTrigger>
            {editing && (
              <TabsTrigger value='audit'>{t('Audit history')}</TabsTrigger>
            )}
          </TabsList>

          <TabsContent
            value='configuration'
            className='min-h-0 overflow-y-auto pb-4'
          >
            {configurationBody}
          </TabsContent>

          {editing && (
            <TabsContent value='audit' className='min-h-0 overflow-y-auto pb-4'>
              {auditLoading && <p>{t('Loading...')}</p>}
              {!auditLoading && auditError && (
                <p role='alert'>{t('Loading failed')}</p>
              )}
              {!auditLoading && !auditError && audits.length === 0 && (
                <Empty className='border'>
                  <EmptyHeader>
                    <EmptyTitle>{t('No contract changes yet')}</EmptyTitle>
                  </EmptyHeader>
                </Empty>
              )}
              {!auditLoading && !auditError && audits.length > 0 && (
                <TemplateAuditList
                  audits={audits}
                  page={auditPage}
                  total={auditTotal}
                  onLoadPage={loadAudits}
                />
              )}
            </TabsContent>
          )}
        </Tabs>

        <SheetFooter className='border-t sm:flex-row sm:justify-between'>
          <Button
            type='button'
            variant='outline'
            disabled={saving}
            onClick={() => props.onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button
            type='button'
            disabled={!dirty || saving || loading}
            onClick={() => void save()}
          >
            {saving ? t('Saving...') : t('Save template')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}

function TemplateAuditList(props: {
  audits: ContractTemplateAudit[]
  page: number
  total: number
  onLoadPage: (page: number) => void
}) {
  const { t } = useTranslation()
  const label = (operation: ContractTemplateAudit['operation']) => {
    switch (operation) {
      case 'create':
        return t('Created')
      case 'enable':
        return t('Enabled')
      case 'disable':
        return t('Disabled')
      default:
        return t('Updated')
    }
  }
  const pageCount = Math.ceil(props.total / 20)
  return (
    <div className='flex flex-col gap-3'>
      {props.audits.map((audit) => (
        <div key={audit.id} className='rounded-lg border p-3'>
          <div className='flex flex-wrap items-center justify-between gap-2'>
            <div className='flex items-center gap-2 font-medium'>
              v{audit.template_version} · {label(audit.operation)}
              <Badge variant={audit.after_enabled ? 'secondary' : 'outline'}>
                {audit.after_enabled ? t('Enabled') : t('Disabled')}
              </Badge>
            </div>
            <div className='text-muted-foreground text-xs'>
              {formatTimestamp(audit.created_at)}
            </div>
          </div>
          <div className='text-muted-foreground mt-1 text-sm'>
            {audit.admin_username || `#${audit.admin_user_id}`} ·{' '}
            {audit.before_rule_count} → {audit.after_rule_count} {t('rules')}
          </div>
          <div className='mt-2 text-sm'>{audit.reason}</div>
        </div>
      ))}
      {pageCount > 1 && (
        <div className='flex items-center justify-end gap-2'>
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={props.page <= 1}
            onClick={() => props.onLoadPage(props.page - 1)}
          >
            {t('Previous')}
          </Button>
          <span className='text-muted-foreground text-sm'>
            {props.page} / {pageCount}
          </span>
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={props.page >= pageCount}
            onClick={() => props.onLoadPage(props.page + 1)}
          >
            {t('Next')}
          </Button>
        </div>
      )}
    </div>
  )
}
