import { Controller, useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { SettingsSwitchField } from '../components/settings-form-layout'

export type MailPreference = {
  id: string
  group: string
  label: string
  recipient: string
  enabled: boolean
}

const groupLabels = {
  channels: 'Channel notifications',
  alerts: 'System and financial alerts',
  reports: 'Daily email reports',
  users: 'User notifications',
} as const

const recipientLabels = {
  operations: 'Report recipients',
  finance: 'Finance recipients',
  root: 'Super administrator notification address',
  subscribers: 'Subscribed administrators',
  account: 'Account email',
  personal: 'Personal notification address',
} as const

export function MailPreferenceFields(props: {
  catalog: MailPreference[]
  hasReportRecipients: boolean
  hasFinanceRecipients: boolean
}) {
  const { t } = useTranslation()
  const form = useFormContext<{ notifications: Record<string, boolean> }>()
  return (
    <section
      className='col-span-full space-y-4'
      aria-labelledby='mail-notification-types'
    >
      <div>
        <h3 id='mail-notification-types' className='font-medium'>
          {t('Notification emails')}
        </h3>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Choose which emails to send. Verification and password reset emails follow account settings.'
          )}
        </p>
      </div>
      {Object.entries(groupLabels).map(([group, label]) => (
        <fieldset key={group} className='min-w-0 space-y-2'>
          <legend className='text-sm font-medium'>{t(label)}</legend>
          {props.catalog
            .filter((entry) => entry.group === group)
            .map((entry) => (
              <Controller
                key={entry.id}
                name={`notifications.${entry.id}`}
                control={form.control}
                render={({ field }) => {
                  const recipient =
                    recipientLabels[
                      entry.recipient as keyof typeof recipientLabels
                    ]
                  const missing =
                    (entry.recipient === 'operations' &&
                      !props.hasReportRecipients) ||
                    (entry.recipient === 'finance' &&
                      !props.hasFinanceRecipients)
                  return (
                    <SettingsSwitchField
                      controlId={`mail-${entry.id}`}
                      label={t(entry.label)}
                      description={
                        missing && field.value
                          ? t('Enabled, but no recipients are configured')
                          : t(recipient)
                      }
                      checked={field.value ?? entry.enabled}
                      onCheckedChange={field.onChange}
                      disabled={form.formState.isSubmitting}
                    />
                  )
                }}
              />
            ))}
        </fieldset>
      ))}
    </section>
  )
}
