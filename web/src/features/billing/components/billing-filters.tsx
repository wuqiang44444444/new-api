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
import { useTranslation } from 'react-i18next'

import { Card, CardContent } from '@/components/ui/card'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import type { BillingPeriod, BillingPreset } from '../types'
import { BillingDateRangePicker } from './billing-date-range-picker'

const ALL_FILTER_VALUE = '__billing_all__'

type FilterOption = {
  value: string
  label: string
}

type BillingFiltersProps = {
  preset: BillingPreset
  period: BillingPeriod
  tokenId?: number
  model?: string
  keyOptions: FilterOption[]
  modelOptions: FilterOption[]
  onPresetChange: (preset: BillingPreset) => void
  onPeriodChange: (startTime: number, endTime: number) => void
  onTokenChange: (tokenId?: number) => void
  onModelChange: (model?: string) => void
}

export function BillingFilters(props: BillingFiltersProps) {
  const { t } = useTranslation()
  const presetOptions: FilterOption[] = [
    { value: 'this-week', label: t('This Week') },
    { value: 'last-week', label: t('Last Week') },
    { value: 'last-7-days', label: t('Last 7 Days') },
    { value: 'last-30-days', label: t('Last 30 Days') },
    { value: 'custom', label: t('Custom Range') },
  ]
  const keyFilterOptions = [
    { value: ALL_FILTER_VALUE, label: t('All') },
    ...props.keyOptions,
  ]
  const modelFilterOptions = [
    { value: ALL_FILTER_VALUE, label: t('All') },
    ...props.modelOptions,
  ]
  const presetLabel =
    presetOptions.find((option) => option.value === props.preset)?.label ??
    t('This Week')
  const selectedKeyValue =
    props.tokenId == null ? ALL_FILTER_VALUE : String(props.tokenId)
  const selectedKeyLabel =
    keyFilterOptions.find((option) => option.value === selectedKeyValue)
      ?.label ?? selectedKeyValue
  const selectedModelValue = props.model || ALL_FILTER_VALUE
  const selectedModelLabel =
    modelFilterOptions.find((option) => option.value === selectedModelValue)
      ?.label ?? selectedModelValue

  return (
    <Card size='sm'>
      <CardContent className='grid gap-3 sm:grid-cols-2 xl:grid-cols-[180px_minmax(300px,1.4fr)_minmax(190px,1fr)_minmax(190px,1fr)]'>
        <label className='space-y-1.5'>
          <span className='text-muted-foreground text-xs font-medium'>
            {t('Billing Period')}
          </span>
          <Select
            items={presetOptions}
            value={props.preset}
            onValueChange={(value) =>
              value != null && props.onPresetChange(value as BillingPreset)
            }
          >
            <SelectTrigger className='w-full'>
              <SelectValue>{presetLabel}</SelectValue>
            </SelectTrigger>
            <SelectContent align='start'>
              <SelectGroup>
                {presetOptions.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </label>

        <div className='space-y-1.5'>
          <span className='text-muted-foreground block text-xs font-medium'>
            {t('Statement Range')}
          </span>
          <BillingDateRangePicker
            startTimestamp={props.period.start_timestamp * 1000}
            endTimestamp={props.period.end_timestamp * 1000}
            onApply={props.onPeriodChange}
          />
        </div>

        <label className='space-y-1.5'>
          <span className='text-muted-foreground text-xs font-medium'>
            {t('API Key')}
          </span>
          <Select
            items={keyFilterOptions}
            value={selectedKeyValue}
            onValueChange={(value) =>
              props.onTokenChange(
                value == null || value === ALL_FILTER_VALUE
                  ? undefined
                  : Number(value)
              )
            }
          >
            <SelectTrigger className='w-full'>
              <SelectValue>{selectedKeyLabel}</SelectValue>
            </SelectTrigger>
            <SelectContent align='start'>
              <SelectGroup>
                {keyFilterOptions.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </label>

        <label className='space-y-1.5'>
          <span className='text-muted-foreground text-xs font-medium'>
            {t('Model')}
          </span>
          <Select
            items={modelFilterOptions}
            value={selectedModelValue}
            onValueChange={(value) =>
              props.onModelChange(
                value == null || value === ALL_FILTER_VALUE ? undefined : value
              )
            }
          >
            <SelectTrigger className='w-full'>
              <SelectValue>{selectedModelLabel}</SelectValue>
            </SelectTrigger>
            <SelectContent align='start'>
              <SelectGroup>
                {modelFilterOptions.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </label>
      </CardContent>
    </Card>
  )
}
