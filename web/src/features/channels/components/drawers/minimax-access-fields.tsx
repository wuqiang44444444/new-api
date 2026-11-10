import { useQuery } from '@tanstack/react-query'
import { useId } from 'react'
import { useFormContext, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Combobox } from '@/components/ui/combobox'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'

import {
  CHANNEL_TYPE_MINIMAX_LINK,
  MINIMAX_JD_VIDEO_PROTOCOL,
  MINIMAX_NEXHINA_VIDEO_PROTOCOL,
  MINIMAX_OFFICIAL_VIDEO_PROTOCOL,
} from '../../constants'
import type { ChannelFormValues } from '../../lib/channel-form'
import {
  MINIMAX_NATIVE_TYPE,
  minimaxAccessLabel,
  resetMinimaxConnectionDraft,
} from '../../lib/minimax-management'
import {
  findMinimaxVideoDeclaration,
  getMinimaxPluginConfiguration,
} from '../../lib/minimax-plugin-configuration'
import { MinimaxProtocolFields } from './minimax-protocol-fields'

export function MinimaxAccessFields(props: {
  editing: boolean
  disabled: boolean
  canSaveDisabled?: boolean
}) {
  const { t } = useTranslation()
  const formId = useId()
  const form = useFormContext<ChannelFormValues>()
  const [type, selected, settings, protocol] = useWatch({
    control: form.control,
    name: [
      'type',
      'minimax_access_selected',
      'settings',
      'video_upstream_protocol',
    ],
  })
  const isStandardVideo = type === CHANNEL_TYPE_MINIMAX_LINK
  const needsDeclaration = !props.editing || isStandardVideo
  // Each open editor keeps the same declaration for its save attempt.
  const configuration = useQuery({
    queryKey: ['minimax-channel-configuration', formId],
    queryFn: getMinimaxPluginConfiguration,
    enabled: needsDeclaration,
    staleTime: 'static',
    gcTime: 0,
    retry: false,
    meta: { errorToast: false },
  })
  const snapshot = configuration.data
  const accessLabel = minimaxAccessLabel({ type, settings: settings ?? '' })
  const videoOptions = [
    { value: MINIMAX_JD_VIDEO_PROTOCOL, label: t('JD Cloud · Standard video') },
    {
      value: MINIMAX_OFFICIAL_VIDEO_PROTOCOL,
      label: t('Official · Standard video'),
    },
    {
      value: MINIMAX_NEXHINA_VIDEO_PROTOCOL,
      label: t('Nexhina · Standard video'),
    },
  ] as const
  const options = [
    ...(props.editing
      ? []
      : [{ value: String(MINIMAX_NATIVE_TYPE), label: t('Native API') }]),
    ...videoOptions.map((option) => {
      const supported = Boolean(
        findMinimaxVideoDeclaration(snapshot, option.value)
      )
      return {
        ...option,
        disabled: !supported,
        description:
          snapshot && !supported
            ? t('Unavailable in plugin version {{version}}', {
                version: snapshot.version,
              })
            : undefined,
      }
    }),
  ]

  let description = t(
    'Choose an access method before configuring the connection.'
  )
  if (isStandardVideo) {
    description = t('Client video endpoint: {{path}}', {
      path: '/api/v3/contents/generations/tasks',
    })
  } else if (selected !== false) {
    description = t(
      'Uses native MiniMax APIs. Available operations depend on the configured models and plugins.'
    )
  }

  let declarationMessage: string | undefined
  if (
    isStandardVideo &&
    !videoOptions.some((option) => option.value === protocol)
  ) {
    declarationMessage = t(
      'This channel has no recognized MiniMax video protocol.'
    )
  } else if (needsDeclaration && configuration.isError) {
    declarationMessage = t('Could not load the MiniMax plugin declaration.')
  } else if (
    isStandardVideo &&
    snapshot &&
    !findMinimaxVideoDeclaration(snapshot, protocol)
  ) {
    declarationMessage = t(
      'The active MiniMax plugin version does not support this access method.'
    )
  }

  let selectedValue = ''
  if (selected !== false) {
    selectedValue = isStandardVideo ? (protocol ?? '') : String(type)
  }

  return (
    <>
      {props.editing && !isStandardVideo ? (
        <dl>
          <dt className='text-sm font-medium'>{t('Access method')}</dt>
          <dd className='mt-2 text-sm'>{t(accessLabel)}</dd>
          <dd className='text-muted-foreground mt-2 space-y-1 text-sm'>
            <p>{description}</p>
            <p>{t('To use another access method, create a new channel.')}</p>
          </dd>
        </dl>
      ) : (
        <FormField
          control={form.control}
          name={
            props.editing
              ? 'video_upstream_protocol'
              : 'minimax_access_selected'
          }
          render={() => (
            <FormItem>
              <FormLabel>{t('Access method')}</FormLabel>
              <FormControl>
                <Combobox
                  options={options}
                  value={selectedValue}
                  aria-label={t('Access method')}
                  placeholder={t('Select access method')}
                  disabled={props.disabled}
                  onValueChange={(value) => {
                    if (props.disabled || value === selectedValue) return
                    if (value === String(MINIMAX_NATIVE_TYPE)) {
                      resetMinimaxConnectionDraft(
                        form,
                        MINIMAX_NATIVE_TYPE,
                        true
                      )
                      return
                    }
                    const option = videoOptions.find(
                      (item) => item.value === value
                    )
                    if (
                      option &&
                      findMinimaxVideoDeclaration(snapshot, option.value)
                    ) {
                      if (props.editing) {
                        form.setValue('video_upstream_protocol', option.value, {
                          shouldDirty: true,
                          shouldValidate: true,
                        })
                        if (
                          option.value ===
                          form.formState.defaultValues?.video_upstream_protocol
                        ) {
                          form.setValue('save_minimax_as_disabled', false)
                        }
                        return
                      }
                      resetMinimaxConnectionDraft(
                        form,
                        CHANNEL_TYPE_MINIMAX_LINK,
                        true,
                        option.value
                      )
                    }
                  }}
                />
              </FormControl>
              <FormDescription className='space-y-1'>
                <span className='block'>{description}</span>
                <span className='block'>
                  {props.editing
                    ? t(
                        'The new access method applies to future tasks. Check the API URL and key before saving.'
                      )
                    : t(
                        'Changing access method clears connection credentials, models and provider settings in this draft.'
                      )}
                </span>
                {configuration.isPending && (
                  <span className='block'>
                    {t('Loading MiniMax access methods…')}
                  </span>
                )}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
      )}
      {declarationMessage && (
        <div role='alert' className='text-destructive space-y-2 text-sm'>
          <p>{declarationMessage}</p>
          {configuration.isError && (
            <Button
              type='button'
              variant='outline'
              size='sm'
              disabled={configuration.isFetching}
              onClick={() => void configuration.refetch()}
            >
              {t('Retry')}
            </Button>
          )}
        </div>
      )}
      {snapshot?.enabled === false &&
        (isStandardVideo || selected === false) && (
          <p role='alert' className='text-destructive text-sm'>
            {t(
              'The MiniMax plugin is disabled. Enable it before enabling this channel.'
            )}
          </p>
        )}
      {isStandardVideo && (
        <>
          {props.editing &&
            props.canSaveDisabled &&
            protocol !==
              form.formState.defaultValues?.video_upstream_protocol && (
              <FormField
                control={form.control}
                name='save_minimax_as_disabled'
                render={({ field }) => (
                  <FormItem>
                    <div className='flex items-center gap-2'>
                      <FormControl>
                        <Checkbox
                          checked={field.value === true}
                          disabled={props.disabled}
                          onCheckedChange={(checked) =>
                            field.onChange(checked === true)
                          }
                        />
                      </FormControl>
                      <FormLabel>{t('Save as disabled')}</FormLabel>
                    </div>
                    <FormDescription>
                      {t(
                        'If pricing is incompatible, save as disabled. Update pricing before enabling this channel.'
                      )}
                    </FormDescription>
                  </FormItem>
                )}
              />
            )}
          <p className='text-muted-foreground text-sm'>
            {t(
              'Set a tiered billing expression for each customer model before sending requests.'
            )}
          </p>
          <MinimaxProtocolFields snapshot={snapshot} />
        </>
      )}
    </>
  )
}
