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
import { AlertTriangle } from 'lucide-react'
import { type Control, useFormContext, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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

import {
  ASSET_TENANT_BOUNDARY_FIELD_LABELS,
  type AssetTenantBoundaryChange,
} from '../../lib/asset-tenant-boundary'
import type { ChannelFormValues } from '../../lib/channel-form'
import { maskAssetCredentialHint } from '../../lib/official-channel-connectivity'
import type { SeedancePluginConfiguration } from '../../lib/seedance-plugin-configuration'
import type {
  SeedanceAssetProtocol,
  SeedanceVideoProtocol,
} from '../../lib/seedance-protocol-pairing'

export type SeedanceProtocolFieldsProps = {
  control: Control<ChannelFormValues>
  sensitiveLocked: boolean
  credentialStatus?: {
    configured: boolean
    access_key_id_hint?: string
  }
  boundaryChanges?: AssetTenantBoundaryChange[]
  // Required: every selector and default here is derived from the published
  // plugin declaration. Callers must gate on a loaded snapshot (the configured
  // wrapper owns loading and failure states).
  configuration: SeedancePluginConfiguration
}

export function SeedanceProtocolFields(props: SeedanceProtocolFieldsProps) {
  const { t } = useTranslation()
  const form = useFormContext<ChannelFormValues>()
  const videoProtocol = useWatch({
    control: props.control,
    name: 'video_upstream_protocol',
  })
  const assetProtocol = useWatch({
    control: props.control,
    name: 'asset_upstream_protocol',
  })
  const models = useWatch({ control: props.control, name: 'models' })
  const videoConfiguration = props.configuration.videos.find(
    (video) => video.protocol === videoProtocol
  )
  const assetConfiguration = props.configuration.assets.find(
    (asset) => asset.protocol === assetProtocol
  )
  const videoOptions = props.configuration.videos.map((video) => ({
    value: video.protocol,
    labelKey: video.label,
  }))
  const assetOptions = props.configuration.assets.map((asset) => ({
    value: asset.protocol,
    labelKey: asset.label,
  }))
  const usesAssets = assetProtocol && assetProtocol !== 'none'
  const usesHostedAssets = assetConfiguration?.groupPolicy === 'hosted'
  const usesOfficialAssets = assetConfiguration?.credential === 'asset_key_pair'
  const usesVolcengineAssets =
    assetProtocol === 'volcengine_assets_action_v2024_01_01'
  const usesCMCCAssets = assetProtocol === 'cmcc_aicc_assets_v2'
  const compatibleAssetProtocols = videoConfiguration?.assetProtocols ?? []
  const compatibleAssetOptions = assetOptions.filter((option) =>
    compatibleAssetProtocols.includes(option.value as SeedanceAssetProtocol)
  )
  let newOfficialCredentialDescription = t(
    'Used only for BytePlus official asset operations.'
  )
  if (usesCMCCAssets) {
    newOfficialCredentialDescription = t(
      'Used only for CMCC Mobile Cloud AICC asset operations.'
    )
  } else if (usesVolcengineAssets) {
    newOfficialCredentialDescription = t(
      'Used only for Volcengine official asset operations.'
    )
  }

  return (
    <>
      {usesAssets ? (
        <Alert>
          <AlertTitle>{t('Asset library boundary: this channel')}</AlertTitle>
          <AlertDescription className='space-y-1'>
            <p>
              {t(
                "One Seedance channel represents one upstream asset tenant. Put every model that must share assets in this channel; a confirmed boundary change replaces this channel's tenant."
              )}
            </p>
            <p>
              {t('Models sharing this asset library: {{models}}', {
                models:
                  models
                    ?.split(',')
                    .map((model) => model.trim())
                    .filter(Boolean)
                    .join(', ') || t('None selected'),
              })}
            </p>
          </AlertDescription>
        </Alert>
      ) : null}

      {props.boundaryChanges?.length ? (
        <Alert className='border-warning/40 bg-warning/5'>
          <AlertTriangle className='text-warning size-4' aria-hidden='true' />
          <AlertTitle>
            {t('This save will replace the asset tenant')}
          </AlertTitle>
          <AlertDescription className='space-y-2'>
            <p>
              {t(
                'Existing asset IDs and asset references may become unavailable. The customer models and channel ID will remain unchanged.'
              )}
            </p>
            <ul className='space-y-1'>
              {props.boundaryChanges.map((change) => (
                <li key={change.field} className='break-words'>
                  <span className='font-medium'>
                    {t(ASSET_TENANT_BOUNDARY_FIELD_LABELS[change.field])}:
                  </span>{' '}
                  <span className='font-mono text-xs'>
                    {change.previous || t('Not set')} →{' '}
                    {change.next || t('Not set')}
                  </span>
                </li>
              ))}
            </ul>
          </AlertDescription>
        </Alert>
      ) : null}

      <FormField
        control={props.control}
        name='video_upstream_protocol'
        render={({ field }) => {
          const selectedOption = videoOptions.find(
            (option) => option.value === field.value
          )

          return (
            <FormItem>
              <FormLabel>{t('Seedance Video Protocol')}</FormLabel>
              <Select
                value={field.value}
                onValueChange={(value) => {
                  const nextVideoProtocol = value as SeedanceVideoProtocol
                  const nextAssetProtocol =
                    props.configuration.videos.find(
                      (video) => video.protocol === nextVideoProtocol
                    )?.defaultAssetProtocol ?? 'none'
                  const nextAssetConfiguration =
                    props.configuration.assets.find(
                      (asset) => asset.protocol === nextAssetProtocol
                    )
                  field.onChange(nextVideoProtocol)
                  form.setValue('asset_upstream_protocol', nextAssetProtocol, {
                    shouldDirty: true,
                    shouldValidate: true,
                  })
                  form.setValue(
                    'asset_min_url_ttl_seconds',
                    nextAssetConfiguration?.defaultURLTTLSeconds ?? 0,
                    { shouldDirty: true, shouldValidate: true }
                  )
                  form.setValue(
                    'asset_provider_project',
                    nextAssetConfiguration?.project?.fixed ??
                      nextAssetConfiguration?.project?.default ??
                      '',
                    {
                      shouldDirty: true,
                      shouldValidate: true,
                    }
                  )
                  form.setValue(
                    'asset_region',
                    nextAssetConfiguration?.region?.fixed ??
                      nextAssetConfiguration?.region?.default ??
                      '',
                    { shouldDirty: true, shouldValidate: true }
                  )
                  form.setValue('asset_access_key_id', '', {
                    shouldDirty: true,
                    shouldValidate: true,
                  })
                  form.setValue('asset_secret_access_key', '', {
                    shouldDirty: true,
                    shouldValidate: true,
                  })
                }}
                disabled={props.sensitiveLocked}
              >
                <FormControl>
                  <SelectTrigger className='w-full max-w-2xl'>
                    <SelectValue placeholder={t('Select video protocol')}>
                      {selectedOption ? t(selectedOption.labelKey) : undefined}
                    </SelectValue>
                  </SelectTrigger>
                </FormControl>
                <SelectContent
                  alignItemWithTrigger={false}
                  className='max-w-[calc(100vw-2rem)] min-w-80 sm:min-w-[36rem]'
                >
                  <SelectGroup>
                    {videoOptions.map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        {t(option.labelKey)}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
              <FormDescription>
                {t(
                  'Choose the code-backed protocol approved by technical staff. Request paths and field conversion are built in.'
                )}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )
        }}
      />

      <FormField
        control={props.control}
        name='asset_upstream_protocol'
        render={({ field }) => {
          const selectedOption = assetOptions.find(
            (option) => option.value === field.value
          )

          return (
            <FormItem>
              <FormLabel>{t('Seedance Asset Protocol')}</FormLabel>
              <Select
                value={field.value || 'none'}
                onValueChange={(value) => {
                  const nextAssetProtocol = value as SeedanceAssetProtocol
                  field.onChange(nextAssetProtocol)
                  const declared = props.configuration.assets.find(
                    (asset) => asset.protocol === nextAssetProtocol
                  )
                  if (declared) {
                    if (!declared.defaultURLTTLSeconds) {
                      form.setValue('asset_min_url_ttl_seconds', 0)
                    } else if (!form.getValues('asset_min_url_ttl_seconds')) {
                      form.setValue(
                        'asset_min_url_ttl_seconds',
                        declared.defaultURLTTLSeconds
                      )
                    }
                    form.setValue(
                      'asset_provider_project',
                      declared.project?.fixed ?? declared.project?.default ?? ''
                    )
                    form.setValue(
                      'asset_region',
                      declared.region?.fixed ?? declared.region?.default ?? ''
                    )
                    if (declared.credential !== 'asset_key_pair') {
                      form.setValue('asset_access_key_id', '')
                      form.setValue('asset_secret_access_key', '')
                    }
                    return
                  }
                }}
                disabled={props.sensitiveLocked}
              >
                <FormControl>
                  <SelectTrigger className='w-full max-w-2xl'>
                    <SelectValue placeholder={t('Select asset protocol')}>
                      {selectedOption ? t(selectedOption.labelKey) : undefined}
                    </SelectValue>
                  </SelectTrigger>
                </FormControl>
                <SelectContent
                  alignItemWithTrigger={false}
                  className='max-w-[calc(100vw-2rem)] min-w-80 sm:min-w-[36rem]'
                >
                  <SelectGroup>
                    {compatibleAssetOptions.map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        {t(option.labelKey)}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
              <FormDescription>
                {t(
                  'Choose one asset library for this channel, or choose none. Assets never switch to another channel.'
                )}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )
        }}
      />

      {usesAssets && !usesHostedAssets ? (
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
                  disabled={props.sensitiveLocked}
                  value={field.value || ''}
                  onChange={(event) =>
                    field.onChange(Number(event.target.value))
                  }
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      ) : null}

      {usesOfficialAssets ? (
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
                    : newOfficialCredentialDescription}
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
                <FormMessage />
              </FormItem>
            )}
          />
        </>
      ) : null}
      {assetConfiguration?.project ? (
        <FormField
          control={props.control}
          name='asset_provider_project'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Provider Project')}</FormLabel>
              <FormControl>
                <Input
                  disabled={
                    props.sensitiveLocked ||
                    Boolean(assetConfiguration?.project?.fixed)
                  }
                  value={field.value || ''}
                  onChange={field.onChange}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      ) : null}
      {assetConfiguration?.region ? (
        <FormField
          control={props.control}
          name='asset_region'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Provider Region')}</FormLabel>
              <FormControl>
                <Input
                  placeholder={
                    assetConfiguration?.region?.fixed ??
                    assetConfiguration?.region?.default ??
                    ''
                  }
                  disabled={
                    props.sensitiveLocked ||
                    Boolean(assetConfiguration?.region?.fixed)
                  }
                  value={field.value || ''}
                  onChange={field.onChange}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      ) : null}
    </>
  )
}
