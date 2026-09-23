import { Trash2 } from 'lucide-react'
import { useId } from 'react'
import { useTranslation } from 'react-i18next'

import { ContractPriceDetails } from '@/components/contract-price-details'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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

import type {
  ContractRuleDraft,
  CustomerContractChannelGroupOption,
} from '../types'
import {
  channelOptionsForRule,
  draftEffectiveMultiplier,
  draftPricePreview,
  normalizeContractDiscount,
  parseContractDiscount,
} from './user-contract-utils'

type CustomerContractRuleListProps = {
  rules: ContractRuleDraft[]
  channelGroups: CustomerContractChannelGroupOption[]
  search: string
  onSearchChange: (value: string) => void
  onUpdate: (index: number, patch: Partial<ContractRuleDraft>) => void
  onRemove: (index: number) => void
}

export function CustomerContractRuleList(props: CustomerContractRuleListProps) {
  const { t } = useTranslation()
  const id = useId()
  return (
    <div className='flex flex-col gap-3'>
      <Input
        value={props.search}
        onChange={(event) => props.onSearchChange(event.target.value)}
        placeholder={t('Search models...')}
        aria-label={t('Search models')}
      />
      {props.rules.map((rule, index) => {
        if (
          props.search &&
          !rule.model.toLowerCase().includes(props.search.toLowerCase())
        ) {
          return null
        }
        const channelOptions = channelOptionsForRule(props.channelGroups, rule)
        const groupRatio = rule.native_group_ratio
        const specialRatio = rule.special_group_ratio
        let unavailableReason = ''
        if (!rule.available) {
          switch (rule.unavailable_reason) {
            case 'channel_disabled':
              unavailableReason = t('This channel is disabled')
              break
            case 'channel_missing':
              unavailableReason = t('This channel no longer exists')
              break
            case 'capability_missing':
              unavailableReason = t(
                'This channel no longer serves this model in the route group'
              )
              break
            case 'route_group_invalid':
              unavailableReason = t('This route group is no longer configured')
              break
            default:
              unavailableReason = t(
                'This source is unavailable. Review its channel, model, and route group.'
              )
          }
        }
        return (
          <div
            key={`${rule.route_group}-${rule.model}-${rule.channel_id}`}
            className='grid gap-3 rounded-lg border p-3 lg:grid-cols-[minmax(180px,1fr)_140px_190px_150px_minmax(220px,1fr)_auto] lg:items-end'
          >
            <Field>
              <FieldLabel>{t('Public model')}</FieldLabel>
              <div className='flex h-8 items-center gap-2 font-mono text-sm'>
                <span className='truncate'>{rule.model}</span>
                <Badge variant={rule.available ? 'secondary' : 'destructive'}>
                  {rule.available ? t('Available') : t('Unavailable')}
                </Badge>
              </div>
            </Field>
            <Field>
              <FieldLabel>{t('Route group')}</FieldLabel>
              <div className='flex h-8 items-center gap-2 font-mono text-sm'>
                {rule.route_group}
              </div>
              {!channelOptions.length && (
                <FieldDescription>
                  {t('No qualifying channel is available for this model')}
                </FieldDescription>
              )}
            </Field>
            <Field>
              <FieldLabel htmlFor={`${id}-channel-${index}`}>
                {t('Channel')}
              </FieldLabel>
              <Select
                items={channelOptions.map((channel) => ({
                  value: String(channel.id),
                  label: channel.name,
                }))}
                value={String(rule.channel_id || '')}
                onValueChange={(value) => {
                  if (!value) return
                  props.onUpdate(index, {
                    channel_id: Number(value),
                    available: true,
                    unavailable_reason: undefined,
                  })
                }}
              >
                <SelectTrigger
                  id={`${id}-channel-${index}`}
                  className='w-full'
                  aria-describedby={
                    unavailableReason ? `${id}-reason-${index}` : undefined
                  }
                >
                  <SelectValue>
                    {rule.channel_id
                      ? channelOptions.find(
                          (channel) => channel.id === rule.channel_id
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
              {unavailableReason && (
                <FieldDescription id={`${id}-reason-${index}`}>
                  {unavailableReason}
                </FieldDescription>
              )}
              {!rule.channel_id && channelOptions.length > 1 && (
                <FieldDescription>
                  {t('Select the channel that serves this model')}
                </FieldDescription>
              )}
            </Field>
            <Field>
              <FieldLabel htmlFor={`contract-discount-${index}`}>
                {t('Contract discount')}
              </FieldLabel>
              <Input
                id={`contract-discount-${index}`}
                value={rule.discount}
                onChange={(event) =>
                  props.onUpdate(index, { discount: event.target.value })
                }
                onBlur={(event) => {
                  const normalized = normalizeContractDiscount(
                    event.target.value
                  )
                  if (normalized !== null) {
                    props.onUpdate(index, { discount: normalized })
                  }
                }}
                placeholder={t('e.g. 0.8, 80%, or 8折')}
                aria-invalid={parseContractDiscount(rule.discount) === null}
              />
              {parseContractDiscount(rule.discount) === null && (
                <FieldDescription className='text-destructive'>
                  {t('Invalid contract discount')}
                </FieldDescription>
              )}
            </Field>
            <Field>
              <FieldLabel>{t('Pricing details')}</FieldLabel>
              <ContractPriceDetails
                price={draftPricePreview(rule)}
                channelMultiplier={groupRatio}
                contractDiscount={rule.discount}
                effectiveMultiplier={draftEffectiveMultiplier(rule)}
              />
              {specialRatio && (
                <FieldDescription>
                  {t('A special native group ratio also applies')}
                </FieldDescription>
              )}
            </Field>
            <Button
              type='button'
              variant='ghost'
              size='icon'
              aria-label={t('Remove contract rule')}
              onClick={() => props.onRemove(index)}
            >
              <Trash2 />
            </Button>
          </div>
        )
      })}
    </div>
  )
}
