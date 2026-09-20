import { Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { MultiSelect } from '@/components/multi-select'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'

import type { CustomerContractChannelGroupOption } from '../types'
import { parseContractDiscount } from './user-contract-utils'

type CustomerContractAddRuleProps = {
  channelGroups: CustomerContractChannelGroupOption[]
  group: string
  model: string
  channelIds: string[]
  discount: string
  onGroupChange: (value: string) => void
  onModelChange: (value: string) => void
  onChannelsChange: (values: string[]) => void
  onDiscountChange: (value: string) => void
  onAdd: () => void
}

export function CustomerContractAddRule(props: CustomerContractAddRuleProps) {
  const { t } = useTranslation()
  const selectedGroupModels =
    props.channelGroups.find((group) => group.group === props.group)?.models ||
    []
  // Every model of the group stays selectable: the same public model may be
  // listed on several channels as long as the discount matches.
  const availableModels = selectedGroupModels.map((entry) => entry.model)
  const channelOptions =
    selectedGroupModels.find((entry) => entry.model === props.model)
      ?.channels || []

  return (
    <Field>
      <FieldLabel>{t('Add model rule')}</FieldLabel>
      <div className='grid gap-2 md:grid-cols-[170px_minmax(200px,1fr)_minmax(220px,1fr)_150px_auto] md:items-end'>
        <Field>
          <FieldLabel>{t('Route group')}</FieldLabel>
          <Combobox
            options={props.channelGroups.map((group) => ({
              value: group.group,
              label: group.group,
            }))}
            value={props.group}
            onValueChange={(value) => {
              props.onGroupChange(value || '')
              props.onModelChange('')
              props.onChannelsChange([])
            }}
            placeholder={t('Select route group')}
            emptyText={t('No available model in this group')}
            openOnFocus
          />
        </Field>
        <Field>
          <FieldLabel>{t('Model')}</FieldLabel>
          <Combobox
            options={availableModels.map((model) => ({
              value: model,
              label: model,
            }))}
            value={props.model}
            // The parent's model handler owns the channel selection: it
            // preselects the single candidate or clears the pick. Clearing
            // channels here again would wipe that preselection.
            onValueChange={(value) => props.onModelChange(value || '')}
            placeholder={t('Search and select a model')}
            emptyText={t('No available model in this group')}
            openOnFocus
          />
        </Field>
        <Field>
          <FieldLabel>{t('Channel')}</FieldLabel>
          <MultiSelect
            options={channelOptions.map((channel) => ({
              value: String(channel.id),
              label: channel.name,
            }))}
            selected={props.channelIds}
            onChange={props.onChannelsChange}
            placeholder={t('Select channels')}
            emptyText={t('No available model in this group')}
            selectAll
            maxVisibleChips={2}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor='contract-batch-discount'>
            {t('Contract discount')}
          </FieldLabel>
          <Input
            id='contract-batch-discount'
            value={props.discount}
            onChange={(event) => props.onDiscountChange(event.target.value)}
            aria-invalid={parseContractDiscount(props.discount) === null}
            placeholder={t('e.g. 0.8, 80%, or 8折')}
          />
        </Field>
        <Button type='button' onClick={props.onAdd} disabled={!props.model}>
          <Plus /> {t('Add')}
        </Button>
      </div>
      <FieldDescription>
        {t(
          'One public model may list several channels; every rule of a model must share the same discount.'
        )}
      </FieldDescription>
    </Field>
  )
}
