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
import { type Control, useWatch } from 'react-hook-form'
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

interface VideoUpstreamProfileFieldProps {
  control: Control<ChannelFormValues>
}

// 官方协议内置任务路径（方案 §3.1），不参与渠道路径配置。
const OFFICIAL_CREATE_PATH = '/api/v3/contents/generations/tasks'
const OFFICIAL_QUERY_PATH = '/api/v3/contents/generations/tasks/{task_id}'

// VideoUpstreamProfileField 为 DoubaoVideo 渠道选择视频上游协议（数据面），
// 并在第三方协议下收集创建/查询路径后缀与实时 URL 预览。profile 是唯一协议控制变量。
export function VideoUpstreamProfileField(
  props: VideoUpstreamProfileFieldProps
) {
  const { t } = useTranslation()
  const profile = useWatch({
    control: props.control,
    name: 'video_upstream_profile',
  })
  const planLocked = Boolean(
    useWatch({
      control: props.control,
      name: 'link_implementation_id',
    })
  )
  const baseURL = useWatch({ control: props.control, name: 'base_url' }) || ''
  const createPath =
    useWatch({ control: props.control, name: 'video_upstream_create_path' }) ||
    ''
  const queryTemplate =
    useWatch({
      control: props.control,
      name: 'video_upstream_query_path_template',
    }) || ''

  const isThirdParty =
    profile === 'third_party_relay' ||
    profile === 'third_party_reverse_proxy' ||
    profile === 'third_party_json_video_media_arrays' ||
    profile === 'third_party_funcloud_seedance_v2'
  const trimmedBase = baseURL.trim().replace(/\/+$/, '')
  const baseForPreview = trimmedBase || t('(channel API address)')

  const createURL = isThirdParty
    ? `${trimmedBase}${createPath}`
    : `${baseForPreview}${OFFICIAL_CREATE_PATH}`
  const queryURL = isThirdParty
    ? `${trimmedBase}${queryTemplate}`
    : `${baseForPreview}${OFFICIAL_QUERY_PATH}`

  let descriptionKey =
    'Third-party reverse proxy protocol keeps the Ark-compatible request unchanged. Configure the third-party API address, create path suffix and query path template.'
  if (profile === 'official') {
    descriptionKey =
      'Official protocol uses the native Ark API directly with built-in task paths. Fill the API address and API key with the official upstream values.'
  } else if (profile === 'third_party_relay') {
    descriptionKey =
      'Third-party relay protocol converts the request into a unified media task structure. Configure the third-party API address, create path suffix and query path template.'
  } else if (profile === 'third_party_json_video_media_arrays') {
    descriptionKey =
      'JSON video media-arrays protocol supports typed text, reference image, and reference audio inputs. Configure an HTTPS API address, create path suffix, and query path template.'
  } else if (profile === 'third_party_funcloud_seedance_v2') {
    descriptionKey =
      'FunCloud Seedance 2.0 uses a typed channel adapter protocol. Configure separate Standard and Fast channels with the documented HTTPS paths.'
  }

  return (
    <div className='flex flex-col gap-4'>
      <FormField
        control={props.control}
        name='video_upstream_profile'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('Video Upstream Profile')}</FormLabel>
            <Select
              items={[
                {
                  value: 'official',
                  label: t('Official Protocol'),
                },
                {
                  value: 'third_party_relay',
                  label: t('Third-party Relay Protocol'),
                },
                {
                  value: 'third_party_reverse_proxy',
                  label: t('Third-party Reverse Proxy Protocol'),
                },
                {
                  value: 'third_party_json_video_media_arrays',
                  label: t('JSON Video Media Arrays Protocol'),
                },
                {
                  value: 'third_party_funcloud_seedance_v2',
                  label: t('FunCloud Seedance 2.0 Protocol'),
                },
              ]}
              onValueChange={field.onChange}
              value={field.value}
              disabled={planLocked}
            >
              <FormControl>
                <SelectTrigger>
                  <SelectValue
                    placeholder={t('Select video upstream profile')}
                  />
                </SelectTrigger>
              </FormControl>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  <SelectItem value='official'>
                    {t('Official Protocol')}
                  </SelectItem>
                  <SelectItem value='third_party_relay'>
                    {t('Third-party Relay Protocol')}
                  </SelectItem>
                  <SelectItem value='third_party_reverse_proxy'>
                    {t('Third-party Reverse Proxy Protocol')}
                  </SelectItem>
                  <SelectItem value='third_party_json_video_media_arrays'>
                    {t('JSON Video Media Arrays Protocol')}
                  </SelectItem>
                  <SelectItem value='third_party_funcloud_seedance_v2'>
                    {t('FunCloud Seedance 2.0 Protocol')}
                  </SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
            <FormDescription>{t(descriptionKey)}</FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />

      {isThirdParty && (
        <>
          <FormField
            control={props.control}
            name='video_upstream_create_path'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Create Path Suffix')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder='/v1/media/generations'
                    disabled={planLocked}
                    {...field}
                  />
                </FormControl>
                <FormDescription className='text-xs'>
                  {t(
                    'Must start with a single / and must not contain {task_id}.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={props.control}
            name='video_upstream_query_path_template'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Query Path Template')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder='/v1/media/tasks/{task_id}'
                    disabled={planLocked}
                    {...field}
                  />
                </FormControl>
                <FormDescription className='text-xs'>
                  {t(
                    'Must start with a single / and contain exactly one {task_id}.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </>
      )}

      <div className='border-border bg-muted/30 text-muted-foreground rounded-md border p-3 text-xs leading-relaxed'>
        <div className='text-foreground font-medium'>
          {t('Final URL Preview')}
        </div>
        <div className='mt-1 break-all'>
          <span className='text-muted-foreground font-mono'>POST</span>{' '}
          <span className='font-mono'>{createURL}</span>
        </div>
        <div className='break-all'>
          <span className='text-muted-foreground font-mono'>GET</span>{' '}
          <span className='font-mono'>{queryURL}</span>
        </div>
      </div>
    </div>
  )
}
