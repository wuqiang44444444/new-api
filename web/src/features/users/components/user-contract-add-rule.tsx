import { Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import type { CustomerContractChannelGroupOption } from '../types'
import { parseContractDiscount } from './user-contract-utils'

type CustomerContractAddRuleProps = {
  channelGroups: CustomerContractChannelGroupOption[]
  group: string
  model: string
  channelId: string
  discount: string
  onGroupChange: (value: string) => void
  onModelChange: (value: string) => void
  onChannelChange: (value: string) => void
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
      <div className='grid gap-2 md:grid-cols-[170px_minmax(200px,1fr)_180px_150px_auto] md:items-end'>
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
              props.onChannelChange('')
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
            onValueChange={(value) => {
              props.onModelChange(value || '')
              props.onChannelChange('')
            }}
            placeholder={t('Search and select a model')}
            emptyText={t('No available model in this group')}
            openOnFocus
          />
        </Field>
        <Field>
          <FieldLabel>{t('Channel')}</FieldLabel>
          <Select
            items={channelOptions.map((channel) => ({
              value: String(channel.id),
              label: channel.name,
            }))}
            value={props.channelId}
            onValueChange={(value) =>
              props.onChannelChange(value ? String(value) : '')
            }
          >
            <SelectTrigger className='w-full'>
              <SelectValue>
                {props.channelId
                  ? channelOptions.find(
                      (channel) => String(channel.id) === props.channelId
                    )?.name
                  : t('Select channel')}
              </SelectValue>
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                {channelOptions.map((channel) => (
                  <SelectItem key={channel.id} value={String(channel.id)}>
                    {channel.name}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
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
