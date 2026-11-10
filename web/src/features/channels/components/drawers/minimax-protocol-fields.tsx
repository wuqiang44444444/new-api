import { useEffect } from 'react'
import { useFormContext, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

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
  findMinimaxVideoDeclaration,
  type MinimaxConfigurationSnapshot,
} from '../../lib/minimax-plugin-configuration'

export function MinimaxProtocolFields(props: {
  snapshot: MinimaxConfigurationSnapshot | undefined
}) {
  const { t } = useTranslation()
  const { control, setValue } = useFormContext<ChannelFormValues>()
  const [version, protocol] = useWatch({
    control,
    name: ['minimax_plugin_version', 'video_upstream_protocol'],
  })
  const video = findMinimaxVideoDeclaration(props.snapshot, protocol)
  const declaredVersion = video ? props.snapshot?.version : ''
  useEffect(() => {
    if ((version ?? '') !== declaredVersion) {
      setValue('minimax_plugin_version', declaredVersion, {
        shouldValidate: true,
      })
    }
  }, [version, declaredVersion, setValue])

  if (!video) return null
  const capabilities = (video.models ?? []).map((model) => ({
    key: `model:${model}`,
    label: model,
    metadata: video.modelMetadata?.[model],
  }))
  if (video.defaultModelMetadata) {
    capabilities.unshift({
      key: 'protocol-default',
      label: video.label,
      metadata: video.defaultModelMetadata,
    })
  }
  return (
    <div className='min-w-0 space-y-3'>
      <div className='text-sm font-medium'>
        {t('Declared video capabilities')}
      </div>
      {capabilities.map(({ key, label, metadata }) => {
        return (
          <div key={key} className='space-y-1 text-sm'>
            <div className='font-medium break-all'>{label}</div>
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
            control={control}
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
