import { useFormContext, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Combobox } from '@/components/ui/combobox'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'

import { CHANNEL_TYPE_MINIMAX_LINK } from '../../constants'
import type { ChannelFormValues } from '../../lib/channel-form'
import {
  MINIMAX_NATIVE_TYPE,
  minimaxAccessLabel,
  resetMinimaxConnectionDraft,
} from '../../lib/minimax-management'

export function MinimaxAccessFields(props: {
  editing: boolean
  disabled: boolean
}) {
  const { t } = useTranslation()
  const form = useFormContext<ChannelFormValues>()
  const [type, selected, settings] = useWatch({
    control: form.control,
    name: ['type', 'minimax_access_selected', 'settings'],
  })
  let description = t(
    'Choose an access method before configuring the connection.'
  )
  if (selected !== false) {
    description =
      type === CHANNEL_TYPE_MINIMAX_LINK
        ? t('Client video endpoint: {{path}}', {
            path: '/api/v3/contents/generations/tasks',
          })
        : t(
            'Uses native MiniMax APIs. Available operations depend on the configured models and plugins.'
          )
  }
  return (
    <FormField
      control={form.control}
      name='minimax_access_selected'
      render={() => (
        <FormItem>
          <FormLabel>{t('Access method')}</FormLabel>
          <FormControl>
            {props.editing ? (
              <Input
                readOnly
                value={t(
                  minimaxAccessLabel({ type, settings: settings ?? '' })
                )}
              />
            ) : (
              <Combobox
                options={[
                  {
                    value: String(MINIMAX_NATIVE_TYPE),
                    label: t('Native API'),
                  },
                  {
                    value: String(CHANNEL_TYPE_MINIMAX_LINK),
                    label: t('JD Cloud · Standard video'),
                  },
                ]}
                value={selected === false ? '' : String(type)}
                placeholder={t('Select access method')}
                disabled={props.disabled}
                onValueChange={(value) => {
                  const next = Number(value)
                  if (
                    props.disabled ||
                    (next !== MINIMAX_NATIVE_TYPE &&
                      next !== CHANNEL_TYPE_MINIMAX_LINK)
                  ) {
                    return
                  }
                  if (selected !== false && next === type) return
                  resetMinimaxConnectionDraft(form, next, true)
                }}
              />
            )}
          </FormControl>
          <FormDescription className='space-y-1 break-words'>
            <span className='block'>{description}</span>
            <span className='block'>
              {props.editing
                ? t('To use another access method, create a new channel.')
                : t(
                    'Changing access method clears connection credentials, models and provider settings in this draft.'
                  )}
            </span>
          </FormDescription>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}
