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
import { useQuery } from '@tanstack/react-query'
import { type Control, useFormContext, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import { getLinkImplementations } from '../../api'
import type { ChannelFormValues } from '../../lib/channel-form'

const NO_LINK_IMPLEMENTATION = '__none__'

interface LinkImplementationFieldProps {
  control: Control<ChannelFormValues>
  channelType: number
}

export function LinkImplementationField({
  control,
  channelType,
}: LinkImplementationFieldProps) {
  const { t } = useTranslation()
  const form = useFormContext<ChannelFormValues>()
  const models = useWatch({ control, name: 'models' }) || ''
  const selectedVersion =
    useWatch({ control, name: 'link_implementation_version' }) || ''
  const { data, isLoading } = useQuery({
    queryKey: ['link_implementations'],
    queryFn: getLinkImplementations,
  })
  const configuredModels = new Set(
    models
      .split(',')
      .map((model) => model.trim())
      .filter(Boolean)
  )
  const implementations = (data?.data || []).filter(
    (implementation) =>
      implementation.channel_type === channelType &&
      (configuredModels.size === 0 ||
        implementation.public_skus.some((model) => configuredModels.has(model)))
  )

  return (
    <FormField
      control={control}
      name='link_implementation_id'
      render={({ field }) => (
        <FormItem>
          <FormLabel>{t('Link Implementation')}</FormLabel>
          <Select
            disabled={isLoading}
            items={[
              {
                value: NO_LINK_IMPLEMENTATION,
                label: t('No Link implementation'),
              },
              ...implementations.map((implementation) => ({
                value: implementation.id,
                label: `${implementation.provider} · ${implementation.id}/${implementation.version}`,
              })),
            ]}
            value={field.value || NO_LINK_IMPLEMENTATION}
            onValueChange={(value) => {
              if (value === NO_LINK_IMPLEMENTATION) {
                field.onChange('')
                form.setValue('link_implementation_version', '')
                return
              }
              const implementation = implementations.find(
                (candidate) => candidate.id === value
              )
              field.onChange(value)
              form.setValue(
                'link_implementation_version',
                implementation?.version || ''
              )
            }}
          >
            <FormControl>
              <SelectTrigger>
                <SelectValue placeholder={t('Select Link implementation')} />
              </SelectTrigger>
            </FormControl>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                <SelectItem value={NO_LINK_IMPLEMENTATION}>
                  {t('No Link implementation')}
                </SelectItem>
                {implementations.map((implementation) => (
                  <SelectItem
                    key={`${implementation.id}/${implementation.version}`}
                    value={implementation.id}
                  >
                    {implementation.provider} · {implementation.id}/
                    {implementation.version}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
          <FormDescription>
            {field.value
              ? t(
                  'Execution is pinned to implementation {{id}}/{{version}}. Saving fails if the channel models or protocol settings do not match.',
                  { id: field.value, version: selectedVersion }
                )
              : t(
                  'Channels publishing registered Link models must select an implementation explicitly.'
                )}
          </FormDescription>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}
