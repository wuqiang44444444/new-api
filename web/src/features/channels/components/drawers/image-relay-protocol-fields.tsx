import { type Control, useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import type { ChannelFormValues } from '../../lib/channel-form'

type ImageRelayProtocolFieldsProps = {
  control: Control<ChannelFormValues>
  sensitiveLocked: boolean
}

type ImageRelayProtocol = NonNullable<
  ChannelFormValues['image_upstream_protocol']
>

const FUN_CLOUD_BASE_URL = 'https://mm-internal-cn.leonecloud.com'
const MOXING_BASE_URL = 'https://www.moxing.pro'

const IMAGE_PROTOCOL_OPTIONS = [
  {
    value: 'funcloud_aigc_v2',
    labelKey: 'FunCloud Async Image V2',
    defaultBaseUrl: FUN_CLOUD_BASE_URL,
  },
  {
    value: 'moxing_images_v1',
    labelKey: 'Moxing Images V1',
    defaultBaseUrl: MOXING_BASE_URL,
  },
] as const

export function ImageRelayProtocolFields(props: ImageRelayProtocolFieldsProps) {
  const { t } = useTranslation()
  const form = useFormContext<ChannelFormValues>()

  return (
    <FormField
      control={props.control}
      name='image_upstream_protocol'
      render={({ field }) => {
        const selectedOption = IMAGE_PROTOCOL_OPTIONS.find(
          (option) => option.value === field.value
        )

        return (
          <FormItem>
            <FormLabel>{t('Image Upstream Protocol')}</FormLabel>
            <Select
              value={field.value}
              onValueChange={(value) => {
                const protocol = value as ImageRelayProtocol
                const option = IMAGE_PROTOCOL_OPTIONS.find(
                  (candidate) => candidate.value === protocol
                )
                field.onChange(protocol)
                const currentBaseUrl = form.getValues('base_url')?.trim() || ''
                if (
                  option &&
                  (!currentBaseUrl ||
                    currentBaseUrl === FUN_CLOUD_BASE_URL ||
                    currentBaseUrl === MOXING_BASE_URL)
                ) {
                  form.setValue('base_url', option.defaultBaseUrl, {
                    shouldDirty: true,
                    shouldValidate: true,
                  })
                }
              }}
              disabled={props.sensitiveLocked}
            >
              <FormControl>
                <SelectTrigger className='w-full max-w-2xl'>
                  <SelectValue placeholder={t('Select image protocol')}>
                    {selectedOption ? t(selectedOption.labelKey) : undefined}
                  </SelectValue>
                </SelectTrigger>
              </FormControl>
              <SelectContent
                alignItemWithTrigger={false}
                className='max-w-[calc(100vw-2rem)] min-w-80 sm:min-w-[36rem]'
              >
                <SelectGroup>
                  {IMAGE_PROTOCOL_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {t(option.labelKey)}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            <FormDescription>
              {t(
                'Choose the code-backed image protocol. Models and model mapping remain administrator-managed.'
              )}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )
      }}
    />
  )
}
