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
  FormLabel,
} from '@/components/ui/form'
import { Switch } from '@/components/ui/switch'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

type Values = {
  enabled: boolean
}

// 客户月账单确认与版本固化能力开关（方案 10.5/10.7）：写入必须经后端专用
// 维护事务，与确认发布建立明确先后边界；此处仅提供管理员开关入口。
export function StatementVersionSettingsSection(props: {
  defaultValue: boolean
}) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const schema = useMemo(() => z.object({ enabled: z.boolean() }), [])
  const form = useForm<Values>({
    resolver: zodResolver(schema) as unknown as Resolver<Values>,
    defaultValues: {
      enabled: props.defaultValue,
    },
  })

  async function onSubmit(values: Values) {
    if (values.enabled === props.defaultValue) {
      toast.info(t('No changes to save'))
      return
    }
    const result = await updateOption.mutateAsync({
      key: 'BillingStatementVersionEnabled',
      value: values.enabled,
    })
    if (result.success) {
      form.reset({ enabled: values.enabled })
    }
  }

  return (
    <SettingsSection title={t('Statement Version Confirmation')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending || form.formState.isSubmitting}
            isSaveDisabled={!form.formState.isDirty}
            saveLabel='Save statement version setting'
          />
          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>
                    {t('Enable monthly statement version confirmation')}
                  </FormLabel>
                  <FormDescription>
                    {t(
                      'When enabled, administrators can generate, confirm and freeze customer monthly statement versions, and confirmed customers see frozen amounts on their statements.'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    disabled={updateOption.isPending}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
