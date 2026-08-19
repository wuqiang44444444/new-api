import { Trash2 } from 'lucide-react'
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
  CustomerContractGroupOption,
  CustomerContractRule,
} from '../types'
import {
  draftPricePreview,
  draftEffectiveMultiplier,
  normalizeContractDiscount,
  parseContractDiscount,
} from './user-contract-utils'

type CustomerContractRuleListProps = {
  rules: CustomerContractRule[]
  options: CustomerContractGroupOption[]
  search: string
  onSearchChange: (value: string) => void
  onUpdate: (index: number, patch: Partial<CustomerContractRule>) => void
  onRemove: (index: number) => void
}

export function CustomerContractRuleList({
  rules,
  options,
  search,
  onSearchChange,
  onUpdate,
  onRemove,
}: CustomerContractRuleListProps) {
  const { t } = useTranslation()
  return (
    <div className='flex flex-col gap-3'>
      <Input
        value={search}
        onChange={(event) => onSearchChange(event.target.value)}
        placeholder={t('Search models...')}
        aria-label={t('Search models')}
      />
      {rules.map((rule, index) => {
        if (
          search &&
          !rule.model.toLowerCase().includes(search.toLowerCase())
        ) {
          return null
        }
        const groupModels =
          options.find((option) => option.group === rule.route_group)?.models ||
          []
        return (
          <div
            key={rule.model}
            className='grid gap-3 rounded-lg border p-3 lg:grid-cols-[minmax(200px,1fr)_170px_140px_160px_160px_auto] lg:items-end'
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
              <Select
                items={options.map((option) => ({
                  value: option.group,
                  label: option.group,
                }))}
                value={rule.route_group}
                onValueChange={(value) => {
                  if (!value) return
                  const option = options.find(
                    (candidate) => candidate.group === value
                  )
                  onUpdate(index, {
                    route_group: value,
                    native_group_ratio: option?.native_group_ratio || '1',
                    special_group_ratio: option?.special_group_ratio || false,
                    price: option?.prices?.[rule.model] || rule.price,
                    effective_multiplier: option?.native_group_ratio || '1',
                  })
                }}
              >
                <SelectTrigger className='w-full'>
                  <SelectValue>{rule.route_group}</SelectValue>
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    {options
                      .filter(
                        (option) =>
                          option.group === rule.route_group ||
                          option.models.includes(rule.model)
                      )
                      .map((option) => (
                        <SelectItem key={option.group} value={option.group}>
                          {option.group}
                        </SelectItem>
                      ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
              <FieldDescription>
                {t('Route group')}: {rule.route_group}
              </FieldDescription>
              {!groupModels.includes(rule.model) && (
                <FieldDescription>
                  {t('Model is unavailable in this group')}
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
                  onUpdate(index, { discount: event.target.value })
                }
                onBlur={(event) => {
                  const normalized = normalizeContractDiscount(
                    event.target.value
                  )
                  if (normalized !== null) {
                    onUpdate(index, { discount: normalized })
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
                channelMultiplier={rule.native_group_ratio}
                contractDiscount={rule.discount}
                effectiveMultiplier={draftEffectiveMultiplier(rule)}
              />
              {rule.special_group_ratio && (
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
              onClick={() => onRemove(index)}
            >
              <Trash2 />
            </Button>
          </div>
        )
      })}
    </div>
  )
}
