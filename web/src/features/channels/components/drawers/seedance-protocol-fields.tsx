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
import {
  getCompatibleSeedanceAssetProtocols,
  getDefaultSeedanceAssetProtocol,
  isOfficialSeedanceAssetProtocol,
  type SeedanceAssetProtocol,
  type SeedanceVideoProtocol,
} from '../../lib/seedance-protocol-pairing'

type SeedanceProtocolFieldsProps = {
  control: Control<ChannelFormValues>
  sensitiveLocked: boolean
  credentialStatus?: {
    configured: boolean
    access_key_id_hint?: string
  }
  boundaryChanges?: AssetTenantBoundaryChange[]
}

const SEEDANCE_VIDEO_PROTOCOL_OPTIONS = [
  { value: 'modelark_v3_volcengine', labelKey: 'Volcengine ModelArk V3' },
  { value: 'modelark_v3_byteplus', labelKey: 'BytePlus ModelArk V3' },
  { value: 'modelark_v3_cmcc', labelKey: 'CMCC Mobile Cloud ModelArk V3' },
  {
    value: 'tokensave_media_task_v1',
    labelKey: 'TokenSave Media Task V1',
  },
  { value: 'moxing_media_task_v1', labelKey: 'Moxing Media Task V1' },
  {
    value: 'moxing_modelark_media_v1',
    labelKey: 'Moxing ModelArk Media Task V1',
  },
  { value: 'ark_media_v1', labelKey: 'Ark Media V1' },
  {
    value: 'feicai_videos_v1',
    labelKey: 'Feicai Videos V1 (URL Only, No Asset Library)',
  },
  { value: 'funcloud_seedance', labelKey: 'FunCloud Seedance' },
] as const

const SEEDANCE_ASSET_PROTOCOL_OPTIONS = [
  { value: 'none', labelKey: 'No Asset Protocol' },
  {
    value: 'volcengine_assets_action_v2024_01_01',
    labelKey: 'Volcengine Official Assets',
  },
  {
    value: 'byteplus_assets_action_v2024_01_01',
    labelKey: 'BytePlus Official Assets',
  },
  { value: 'ark_assets_v1', labelKey: 'Ark Assets V1' },
  { value: 'tokensave_assets_v1', labelKey: 'TokenSave Asset Library V1' },
  {
    value: 'moxing_joycreator_assets_v1',
    labelKey: 'Moxing JoyCreator Asset Library V1',
  },
  {
    value: 'moxing_volc_assets_v1',
    labelKey: 'Moxing Volcengine Asset Library V1',
  },
  {
    value: 'funcloud_material',
    labelKey: 'FunCloud Material Library',
  },
  {
    value: 'cmcc_aicc_assets_v2',
    labelKey: 'CMCC AICC Assets V2',
  },
] as const

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
  const modelMapping = useWatch({
    control: props.control,
    name: 'model_mapping',
  })
  const models = useWatch({ control: props.control, name: 'models' })
  const usesAssets = assetProtocol && assetProtocol !== 'none'
  const usesOfficialAssets = isOfficialSeedanceAssetProtocol(assetProtocol)
  const usesVolcengineAssets =
    assetProtocol === 'volcengine_assets_action_v2024_01_01'
  const usesCMCCAssets = assetProtocol === 'cmcc_aicc_assets_v2'
  const compatibleAssetProtocols = getCompatibleSeedanceAssetProtocols(
    videoProtocol,
    modelMapping
  )
  const compatibleAssetOptions = SEEDANCE_ASSET_PROTOCOL_OPTIONS.filter(
    (option) =>
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
          const selectedOption = SEEDANCE_VIDEO_PROTOCOL_OPTIONS.find(
            (option) => option.value === field.value
          )

          return (
            <FormItem>
              <FormLabel>{t('Seedance Video Protocol')}</FormLabel>
              <Select
                value={field.value}
                onValueChange={(value) => {
                  const nextVideoProtocol = value as SeedanceVideoProtocol
                  const nextAssetProtocol = getDefaultSeedanceAssetProtocol(
                    nextVideoProtocol,
                    modelMapping
                  )
                  field.onChange(nextVideoProtocol)
                  form.setValue('asset_upstream_protocol', nextAssetProtocol, {
                    shouldDirty: true,
                    shouldValidate: true,
                  })
                  form.setValue(
                    'asset_min_url_ttl_seconds',
                    nextAssetProtocol === 'none' ? 0 : 3600,
                    { shouldDirty: true, shouldValidate: true }
                  )
                  form.setValue('asset_provider_project', '', {
                    shouldDirty: true,
                    shouldValidate: true,
                  })
                  form.setValue(
                    'asset_region',
                    nextAssetProtocol === 'volcengine_assets_action_v2024_01_01'
                      ? 'cn-beijing'
                      : '',
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
                    {SEEDANCE_VIDEO_PROTOCOL_OPTIONS.map((option) => (
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
          const selectedOption = SEEDANCE_ASSET_PROTOCOL_OPTIONS.find(
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
                  if (nextAssetProtocol === 'none') {
                    form.setValue('asset_min_url_ttl_seconds', 0)
                  } else if (!form.getValues('asset_min_url_ttl_seconds')) {
                    form.setValue('asset_min_url_ttl_seconds', 3600)
                  }
                  if (!isOfficialSeedanceAssetProtocol(nextAssetProtocol)) {
                    form.setValue('asset_provider_project', '')
                    form.setValue('asset_region', '')
                    form.setValue('asset_access_key_id', '')
                    form.setValue('asset_secret_access_key', '')
                  } else if (
                    nextAssetProtocol === 'volcengine_assets_action_v2024_01_01'
                  ) {
                    form.setValue('asset_region', 'cn-beijing')
                  } else if (form.getValues('asset_region') === 'cn-beijing') {
                    form.setValue('asset_region', '')
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

      {usesAssets ? (
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
          {!usesCMCCAssets ? (
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
                  <FormMessage />
                </FormItem>
              )}
            />
          ) : null}
          {!usesCMCCAssets ? (
            <FormField
              control={props.control}
              name='asset_region'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Provider Region')}</FormLabel>
                  <FormControl>
                    <Input
                      placeholder={
                        usesVolcengineAssets ? 'cn-beijing' : 'ap-southeast-1'
                      }
                      disabled={props.sensitiveLocked || usesVolcengineAssets}
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
      ) : null}
    </>
  )
}
