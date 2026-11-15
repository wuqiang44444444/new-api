import { useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'

import { SettingsCard } from '../components/settings-card'
import type { RoutingPolicyFormValues } from './routing-form'

export function FailoverRoutingFields() {
  const { t } = useTranslation()
  const { control } = useFormContext<RoutingPolicyFormValues>()

  return (
    <SettingsCard title={t('Channel failover')}>
      <div className='grid gap-4 sm:grid-cols-2'>
        <FormField
          control={control}
          name='channel_affinity_setting.failover_mode'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Failover mode')}</FormLabel>
              <FormControl>
                <NativeSelect className='w-full' {...field}>
                  <NativeSelectOption value='off'>
                    {t('Off')}
                  </NativeSelectOption>
                  <NativeSelectOption value='observe'>
                    {t('Observe only')}
                  </NativeSelectOption>
                  <NativeSelectOption value='enabled'>
                    {t('Enabled for rollout channels')}
                  </NativeSelectOption>
                </NativeSelect>
              </FormControl>
            </FormItem>
          )}
        />
        <FormField
          control={control}
          name='channel_affinity_setting.failover_channel_ids'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Rollout channel IDs')}</FormLabel>
              <FormControl>
                <Input placeholder='20, 21' {...field} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={control}
          name='channel_affinity_setting.failover_verified_pairs'
          render={({ field }) => (
            <FormItem className='sm:col-span-2'>
              <FormLabel>{t('Verified migration pairs')}</FormLabel>
              <FormControl>
                <Textarea
                  className='font-mono text-xs'
                  rows={4}
                  placeholder='[{"source_channel_id":86,"target_channel_id":231,"model":"claude-opus-4-8"}]'
                  {...field}
                />
              </FormControl>
              <FormDescription>
                {t(
                  'Saving binds each pair to the current channel configuration. A later channel change disables the pair; clear its revisions and save again after re-verification.'
                )}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={control}
          name='channel_affinity_setting.layered_affinity'
          render={({ field }) => (
            <FormItem className='flex items-center gap-3'>
              <FormControl>
                <Switch
                  checked={field.value}
                  onCheckedChange={field.onChange}
                />
              </FormControl>
              <FormLabel>
                {t('Fall back to API Key and user affinity')}
              </FormLabel>
            </FormItem>
          )}
        />
      </div>
    </SettingsCard>
  )
}
