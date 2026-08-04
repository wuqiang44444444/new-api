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
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import type { ChannelFormValues } from '../../lib/channel-form'
import { maskAssetCredentialHint } from '../../lib/official-channel-connectivity'

interface AssetUpstreamProfileFieldProps {
  control: Control<ChannelFormValues>
  sensitiveLocked: boolean
  credentialStatus?: {
    configured: boolean
    access_key_id_hint?: string
  }
}

export function AssetUpstreamProfileField(
  props: AssetUpstreamProfileFieldProps
) {
  const { t } = useTranslation()
  const form = useFormContext<ChannelFormValues>()
  const profile = useWatch({
    control: props.control,
    name: 'asset_upstream_profile',
  })
  const planLocked = Boolean(
    useWatch({
      control: props.control,
      name: 'link_implementation_id',
    })
  )

  let description = t(
    'No upstream asset library is associated with this channel.'
  )
  if (profile === 'ark_assets') {
    description = t(
      'Ark Assets supports ordinary assets and managed real-person verification. It requires the third-party reverse proxy video profile.'
    )
  } else if (profile === 'relay_assets') {
    description = t(
      'Relay Assets supports ordinary routable assets. It requires the third-party relay video profile.'
    )
  } else if (profile === 'joycreator_assets') {
    description = t(
      'JoyCreator Assets is management-only and never participates in video routing.'
    )
  } else if (profile === 'official_action_assets') {
    description = t(
      'Official Action Assets uses separate signed ModelArk Action credentials and requires the official video profile. The channel key remains the video API key.'
    )
  }

  return (
    <>
      <FormField
        control={props.control}
        name='asset_upstream_profile'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('Asset Upstream Profile')}</FormLabel>
            <Select
              items={[
                { value: 'none', label: t('No Asset Protocol') },
                { value: 'ark_assets', label: t('Ark Assets') },
                { value: 'relay_assets', label: t('Relay Assets') },
                { value: 'joycreator_assets', label: t('JoyCreator Assets') },
                {
                  value: 'official_action_assets',
                  label: t('Official Action Assets'),
                },
              ]}
              onValueChange={(value) => {
                field.onChange(value)
                if (value !== 'official_action_assets') {
                  form.setValue('asset_access_key_id', '')
                  form.setValue('asset_secret_access_key', '')
                }
              }}
              value={field.value}
              disabled={props.sensitiveLocked || planLocked}
            >
              <FormControl>
                <SelectTrigger>
                  <SelectValue
                    placeholder={t('Select asset upstream profile')}
                  />
                </SelectTrigger>
              </FormControl>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  <SelectItem value='none'>{t('No Asset Protocol')}</SelectItem>
                  <SelectItem value='ark_assets'>{t('Ark Assets')}</SelectItem>
                  <SelectItem value='relay_assets'>
                    {t('Relay Assets')}
                  </SelectItem>
                  <SelectItem value='joycreator_assets'>
                    {t('JoyCreator Assets')}
                  </SelectItem>
                  <SelectItem value='official_action_assets'>
                    {t('Official Action Assets')}
                  </SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
            <FormDescription>{description}</FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
      {profile !== undefined && profile !== 'none' ? (
        <FormField
          control={props.control}
          name='asset_min_url_ttl_seconds'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Minimum Asset URL TTL (seconds)')}</FormLabel>
              <FormControl>
                <Input
                  type='number'
                  min={1}
                  step={1}
                  disabled={props.sensitiveLocked || planLocked}
                  value={field.value || ''}
                  onChange={(event) =>
                    field.onChange(Number(event.target.value))
                  }
                />
              </FormControl>
              <FormDescription>
                {t(
                  'Verified Provider fetch window, including retries and clock-skew margin. Remote assets stay disabled for this channel when it is zero.'
                )}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
      ) : null}
      {profile === 'official_action_assets' ? (
        <>
          <FormField
            control={props.control}
            name='asset_access_key_id'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Asset Access Key ID')}</FormLabel>
                <FormControl>
                  <Input
                    type='password'
                    autoComplete='new-password'
                    disabled={props.sensitiveLocked}
                    value={field.value || ''}
                    onChange={field.onChange}
                  />
                </FormControl>
                <FormDescription>
                  {props.credentialStatus?.configured
                    ? t(
                        'Configured as {{hint}}. Leave both asset credential fields blank to keep the current credentials.',
                        {
                          hint:
                            maskAssetCredentialHint(
                              props.credentialStatus.access_key_id_hint
                            ) || t('hidden'),
                        }
                      )
                    : t(
                        'Used only to sign official ModelArk asset Actions. It is never returned after saving.'
                      )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={props.control}
            name='asset_secret_access_key'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Asset Secret Access Key')}</FormLabel>
                <FormControl>
                  <Input
                    type='password'
                    autoComplete='new-password'
                    disabled={props.sensitiveLocked}
                    value={field.value || ''}
                    onChange={field.onChange}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Enter both asset credential fields to create or rotate the credential.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={props.control}
            name='asset_provider_project'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Provider Project')}</FormLabel>
                <FormControl>
                  <Input
                    disabled={props.sensitiveLocked}
                    value={field.value || ''}
                    onChange={field.onChange}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'ModelArk resource project. Assets and the video endpoint must use the same project.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={props.control}
            name='asset_region'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Provider Region')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder='ap-southeast-1'
                    disabled={props.sensitiveLocked}
                    value={field.value || ''}
                    onChange={field.onChange}
                  />
                </FormControl>
                <FormDescription>
                  {t('Region used to sign official ModelArk asset Actions.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </>
      ) : null}
    </>
  )
}
