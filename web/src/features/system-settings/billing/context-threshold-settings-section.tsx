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
import { zodResolver } from '@hookform/resolvers/zod'
import { useMemo } from 'react'
import { useForm, type Resolver } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Textarea } from '@/components/ui/textarea'

import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

const maxContextThreshold = 2147483647

function parseThresholds(value: string) {
  const parsed: unknown = JSON.parse(value)
  if (parsed == null || Array.isArray(parsed) || typeof parsed !== 'object') {
    throw new Error('Thresholds must be a JSON object')
  }
  for (const [model, threshold] of Object.entries(parsed)) {
    if (!model || model !== model.trim()) {
      throw new Error('Model names cannot be empty or contain outer spaces')
    }
    if (
      typeof threshold !== 'number' ||
      !Number.isInteger(threshold) ||
      threshold <= 0 ||
      threshold > maxContextThreshold
    ) {
      throw new Error('Thresholds must be positive integers')
    }
  }
  return parsed as Record<string, number>
}

type Values = {
  thresholds: string
}

function prettyThresholds(value: string) {
  try {
    return JSON.stringify(parseThresholds(value), null, 2)
  } catch {
    return value || '{}'
  }
}

export function ContextThresholdSettingsSection(props: {
  defaultValue: string
}) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const schema = useMemo(
    () =>
      z.object({
        thresholds: z.string().superRefine((value, context) => {
          try {
            parseThresholds(value)
          } catch {
            context.addIssue({
              code: 'custom',
              message: t(
                'Enter a valid JSON object with positive integer thresholds.'
              ),
            })
          }
        }),
      }),
    [t]
  )
  const form = useForm<Values>({
    resolver: zodResolver(schema) as unknown as Resolver<Values>,
    defaultValues: {
      thresholds: prettyThresholds(props.defaultValue),
    },
  })

  async function onSubmit(values: Values) {
    const normalized = JSON.stringify(parseThresholds(values.thresholds))
    if (
      normalized === JSON.stringify(parseThresholds(props.defaultValue || '{}'))
    ) {
      toast.info(t('No changes to save'))
      return
    }
    const result = await updateOption.mutateAsync({
      key: 'billing_statement_setting.context_thresholds',
      value: normalized,
    })
    if (result.success) {
      form.reset({ thresholds: prettyThresholds(normalized) })
    }
  }

  return (
    <SettingsSection title={t('Usage Statement Breakdown')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending || form.formState.isSubmitting}
            isSaveDisabled={!form.formState.isDirty}
            saveLabel='Save context thresholds'
          />
          <FormField
            control={form.control}
            name='thresholds'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Model context thresholds')}</FormLabel>
                <FormControl>
                  <Textarea
                    {...field}
                    className='min-h-56 font-mono text-sm'
                    spellCheck={false}
                    placeholder={'{\n  "gpt-5": 128000\n}'}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Enter a JSON object of exact model names and input-token thresholds. Models without a threshold do not show long or short context.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
