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
    profile === 'third_party_relay' || profile === 'third_party_reverse_proxy'
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
              ]}
              onValueChange={field.onChange}
              value={field.value}
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
                  <Input placeholder='/v1/media/generations' {...field} />
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

      <div className='rounded-md border border-border bg-muted/30 p-3 text-xs leading-relaxed text-muted-foreground'>
        <div className='font-medium text-foreground'>{t('Final URL Preview')}</div>
        <div className='mt-1 break-all'>
          <span className='font-mono text-muted-foreground'>POST</span>{' '}
          <span className='font-mono'>{createURL}</span>
        </div>
        <div className='break-all'>
          <span className='font-mono text-muted-foreground'>GET</span>{' '}
          <span className='font-mono'>{queryURL}</span>
        </div>
      </div>
    </div>
  )
}
