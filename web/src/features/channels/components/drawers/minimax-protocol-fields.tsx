import { useQuery } from '@tanstack/react-query'
import { useEffect, useId, useState } from 'react'
import { useFormContext, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'

import type { ChannelFormValues } from '../../lib/channel-form'
import {
  getMinimaxPluginConfiguration,
  type MinimaxConfigurationSnapshot,
} from '../../lib/minimax-plugin-configuration'

// Configuration is pinned for this mounted editor. Modes unmount this component,
// so a late response cannot copy its declaration into another connection draft.
export function MinimaxProtocolFields() {
  const { t } = useTranslation()
  const formId = useId()
  const form = useFormContext<ChannelFormValues>()
  const version = useWatch({
    control: form.control,
    name: 'minimax_plugin_version',
  })
  const [snapshot, setSnapshot] = useState<MinimaxConfigurationSnapshot>()
  const configuration = useQuery({
    queryKey: ['minimax-channel-configuration', formId],
    queryFn: getMinimaxPluginConfiguration,
    enabled: snapshot === undefined,
    staleTime: Infinity,
    gcTime: 0,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
  useEffect(() => {
    if (configuration.data && !snapshot) setSnapshot(configuration.data)
  }, [configuration.data, snapshot])
  useEffect(() => {
    if (snapshot && version !== snapshot.version) {
      form.setValue('minimax_plugin_version', snapshot.version, {
        shouldValidate: true,
      })
    }
  }, [snapshot, version, form])
  if (!snapshot && configuration.isError) {
    return (
      <ErrorState
        description={t('Load the plugin declaration before saving')}
        onRetry={() => {
          void configuration.refetch()
        }}
      />
    )
  }
  if (!snapshot) return <LoadingState />

  const video = snapshot.configuration?.videos.find(
    (item) => item.protocol === 'jdcloud_video_task_v1'
  )
  return (
    <div className='min-w-0 space-y-3'>
      <div className='text-sm font-medium'>
        {t('Declared video capabilities')}
      </div>
      {video?.models.map((model) => {
        const metadata = video.modelMetadata?.[model]
        return (
          <div key={model} className='space-y-1 text-sm'>
            <div className='font-medium break-all'>{model}</div>
            <dl className='text-muted-foreground space-y-1'>
              {metadata?.minDuration != null &&
                metadata.maxDuration != null && (
                  <div>
                    <dt>{t('Video duration')}</dt>
                    <dd>
                      {t('{{min}}–{{max}} seconds', {
                        min: metadata.minDuration,
                        max: metadata.maxDuration,
                      })}
                    </dd>
                  </div>
                )}
              {metadata?.resolutions && (
                <div>
                  <dt>{t('Resolution')}</dt>
                  <dd>{metadata.resolutions.join(', ')}</dd>
                </div>
              )}
              {metadata?.ratios && (
                <div>
                  <dt>{t('Aspect ratio')}</dt>
                  <dd>{metadata.ratios.join(', ')}</dd>
                </div>
              )}
              {metadata?.maxImages != null && (
                <div>
                  <dt>{t('Reference images')}</dt>
                  <dd>{t('Up to {{count}}', { count: metadata.maxImages })}</dd>
                </div>
              )}
              {metadata?.allowVideos && metadata.maxVideos != null && (
                <div>
                  <dt>{t('Reference videos')}</dt>
                  <dd>{t('Up to {{count}}', { count: metadata.maxVideos })}</dd>
                </div>
              )}
              {metadata?.allowAudios && metadata.maxAudios != null && (
                <div>
                  <dt>{t('Reference audio')}</dt>
                  <dd>{t('Up to {{count}}', { count: metadata.maxAudios })}</dd>
                </div>
              )}
              {metadata?.allowFrameImages != null && (
                <div>
                  <dt>{t('First and last frames')}</dt>
                  <dd>
                    {t(
                      metadata.allowFrameImages ? 'Supported' : 'Not supported'
                    )}
                  </dd>
                </div>
              )}
            </dl>
          </div>
        )
      })}
      <Collapsible>
        <CollapsibleTrigger render={<Button variant='ghost' size='sm' />}>
          {t('Plugin declaration version')}
        </CollapsibleTrigger>
        <CollapsibleContent>
          <FormField
            control={form.control}
            name='minimax_plugin_version'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Plugin declaration version')}</FormLabel>
                <FormControl>
                  <Input {...field} value={field.value ?? ''} readOnly />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </CollapsibleContent>
      </Collapsible>
    </div>
  )
}
