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
import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

import { PasswordInput } from '@/components/password-input'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Switch } from '@/components/ui/switch'
import { api } from '@/lib/api'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import {
  SettingsForm,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'

type SmtpConfig = {
  server: string
  port: number
  account: string
  from: string
  token?: string
  ssl: boolean
  start_tls: boolean
  skip_verify: boolean
  force_login: boolean
}
type GraphConfig = {
  tenant_id: string
  client_id: string
  sender: string
  mailbox_id?: string
  client_secret?: string
}
type MailTransport =
  | { mode: 'smtp'; smtp: SmtpConfig }
  | { mode: 'graph'; graph: GraphConfig }
type MailTransportView = { config: MailTransport; has_credential: boolean }

type MailOptions = {
  'error_report_setting.recipients': string
  'resource_alert_setting.cpu': number
  'resource_alert_setting.memory': number
  'resource_alert_setting.disk': number
}

const transportQueryKey = ['mail-transport']
const thresholdFields = [
  ['cpu', 'CPU alert threshold (%)'],
  ['memory', 'Memory alert threshold (%)'],
  ['disk', 'Disk alert threshold (%)'],
] as const

const schema = z.object({
  mode: z.enum(['smtp', 'graph']),
  server: z.string(),
  port: z.string(),
  account: z.string(),
  from: z.string(),
  smtpSecurity: z.enum(['none', 'ssl', 'start_tls']),
  skipVerify: z.boolean(),
  forceLogin: z.boolean(),
  tenantId: z.string(),
  clientId: z.string(),
  sender: z.string(),
  mailboxId: z.string(),
  credential: z.string(),
  recipients: z.string(),
  cpu: z.string(),
  memory: z.string(),
  disk: z.string(),
})
type FormValues = z.infer<typeof schema>
type TextField =
  | 'server'
  | 'port'
  | 'account'
  | 'from'
  | 'tenantId'
  | 'clientId'
  | 'sender'
  | 'mailboxId'
  | 'recipients'
  | 'cpu'
  | 'memory'
  | 'disk'

function toTransport(values: FormValues): MailTransport {
  if (values.mode === 'graph') {
    return {
      mode: 'graph',
      graph: {
        tenant_id: values.tenantId.trim(),
        client_id: values.clientId.trim(),
        sender: values.sender.trim(),
        mailbox_id: values.mailboxId.trim() || undefined,
        client_secret: values.credential,
      },
    }
  }
  return {
    mode: 'smtp',
    smtp: {
      server: values.server.trim(),
      port: Number(values.port),
      account: values.account.trim(),
      from: values.from.trim(),
      token: values.credential,
      ssl: values.smtpSecurity === 'ssl',
      start_tls: values.smtpSecurity === 'start_tls',
      skip_verify: values.skipVerify,
      force_login: values.forceLogin,
    },
  }
}

export function EmailSettingsSection(props: { defaultValues: MailOptions }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const updateOption = useUpdateOption()
  const transport = useQuery({
    queryKey: transportQueryKey,
    queryFn: async () => {
      const result = await api.get('/api/option/mail')
      return requireServerSuccess(result.data).data as MailTransportView
    },
  })
  const config = transport.data?.config
  const smtp = config?.mode === 'smtp' ? config.smtp : undefined
  const graph = config?.mode === 'graph' ? config.graph : undefined
  let smtpSecurity: FormValues['smtpSecurity'] = 'none'
  if (smtp?.ssl) smtpSecurity = 'ssl'
  else if (smtp?.start_tls) smtpSecurity = 'start_tls'
  const defaults: FormValues = {
    mode: config?.mode ?? 'smtp',
    server: smtp?.server ?? '',
    port: String(smtp?.port || 587),
    account: smtp?.account ?? '',
    from: smtp?.from ?? '',
    smtpSecurity,
    skipVerify: smtp?.skip_verify ?? false,
    forceLogin: smtp?.force_login ?? false,
    tenantId: graph?.tenant_id ?? '',
    clientId: graph?.client_id ?? '',
    sender: graph?.sender ?? '',
    mailboxId: graph?.mailbox_id ?? '',
    credential: '',
    recipients: props.defaultValues['error_report_setting.recipients'],
    cpu: String(props.defaultValues['resource_alert_setting.cpu']),
    memory: String(props.defaultValues['resource_alert_setting.memory']),
    disk: String(props.defaultValues['resource_alert_setting.disk']),
  }
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: defaults,
  })
  useResetForm(form, form.formState.isSubmitting ? undefined : defaults)
  const mode = form.watch('mode')

  async function save(values: FormValues) {
    if (!transport.data) return
    const changedTransport = toTransport(values)
    const recipients = values.recipients.trim()
    for (const [metric] of thresholdFields) {
      const value = Number(values[metric])
      if (!Number.isInteger(value) || value < 1 || value > 100) {
        form.setError(metric, {
          message: t(
            'Resource alert threshold must be between 1 and 100 percent'
          ),
        })
        return
      }
    }
    try {
      if (
        JSON.stringify(changedTransport) !==
        JSON.stringify(toTransport(defaults))
      ) {
        const result = await api.put('/api/option/mail', changedTransport)
        const saved = requireServerSuccess(result.data)
          .data as MailTransportView
        queryClient.setQueryData(transportQueryKey, saved)
        toast.success(t('Setting updated successfully'))
      }
      for (const [metric] of thresholdFields) {
        if (values[metric] !== defaults[metric]) {
          await updateOption.mutateAsync({
            key: `resource_alert_setting.${metric}`,
            value: Number(values[metric]),
          })
        }
      }
      if (recipients !== defaults.recipients) {
        await updateOption.mutateAsync({
          key: 'error_report_setting.recipients',
          value: recipients,
        })
      }
    } catch (error) {
      handleServerError(error, t('Failed to update setting'))
    }
  }

  function textField(name: TextField, label: string, type = 'text') {
    return (
      <FormField
        key={name}
        control={form.control}
        name={name}
        render={({ field }) => (
          <FormItem>
            <FormLabel>{label}</FormLabel>
            <FormControl>
              <Input {...field} type={type} autoComplete='off' />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
    )
  }

  return (
    <SettingsSection title={t('Email delivery')}>
      {transport.isError && (
        <Button
          type='button'
          variant='outline'
          onClick={() => transport.refetch()}
        >
          {t('Retry')}
        </Button>
      )}
      {transport.data && (
        <>
          <Form {...form}>
            <SettingsForm onSubmit={form.handleSubmit(save)}>
              <SettingsPageFormActions
                onSave={form.handleSubmit(save)}
                isSaving={form.formState.isSubmitting}
                saveLabel='Save email settings'
              />
              <FormField
                control={form.control}
                name='mode'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Mail transport')}</FormLabel>
                    <FormControl>
                      <RadioGroup
                        value={field.value}
                        onValueChange={(value) => {
                          field.onChange(value)
                          form.setValue('credential', '')
                        }}
                        className='flex gap-4'
                      >
                        <Label>
                          <RadioGroupItem value='smtp' />
                          SMTP
                        </Label>
                        <Label>
                          <RadioGroupItem value='graph' />
                          Microsoft Graph
                        </Label>
                      </RadioGroup>
                    </FormControl>
                  </FormItem>
                )}
              />
              {mode === 'smtp' ? (
                <>
                  {textField('server', t('SMTP Host'))}
                  {textField('port', t('Port'), 'number')}
                  {textField('account', t('Username'))}
                  {textField('from', t('From Address'), 'email')}
                  <FormField
                    control={form.control}
                    name='smtpSecurity'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Connection Security')}</FormLabel>
                        <FormControl>
                          <RadioGroup
                            value={field.value}
                            onValueChange={field.onChange}
                            className='flex gap-4'
                          >
                            <Label>
                              <RadioGroupItem value='none' />
                              {t('None')}
                            </Label>
                            <Label>
                              <RadioGroupItem value='ssl' />
                              SSL/TLS
                            </Label>
                            <Label>
                              <RadioGroupItem value='start_tls' />
                              STARTTLS
                            </Label>
                          </RadioGroup>
                        </FormControl>
                      </FormItem>
                    )}
                  />
                  {(
                    [
                      ['skipVerify', t('Skip TLS certificate verification')],
                      ['forceLogin', t('Force AUTH LOGIN')],
                    ] as const
                  ).map(([name, label]) => (
                    <FormField
                      key={name}
                      control={form.control}
                      name={name}
                      render={({ field }) => (
                        <SettingsSwitchItem>
                          <FormLabel>{label}</FormLabel>
                          <FormControl>
                            <Switch
                              checked={field.value}
                              onCheckedChange={field.onChange}
                            />
                          </FormControl>
                        </SettingsSwitchItem>
                      )}
                    />
                  ))}
                </>
              ) : (
                <>
                  {textField('tenantId', t('Tenant ID'))}
                  {textField('clientId', t('Client ID'))}
                  {textField('sender', t('Dedicated sender mailbox'), 'email')}
                  {textField('mailboxId', t('Mailbox object ID (optional)'))}
                </>
              )}
              <FormField
                control={form.control}
                name='credential'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {mode === 'graph'
                        ? t('Client Secret')
                        : t('Password / Access Token')}
                    </FormLabel>
                    <FormControl>
                      <PasswordInput {...field} autoComplete='new-password' />
                    </FormControl>
                    {transport.data.has_credential && config?.mode === mode && (
                      <FormDescription>
                        {t('Leave blank to keep the existing credential')}
                      </FormDescription>
                    )}
                  </FormItem>
                )}
              />
              {textField('recipients', t('Report recipients'))}
              <p className='text-muted-foreground text-sm'>
                {t(
                  'Daily reports and operations alerts are sent automatically to these addresses. Separate addresses with commas or semicolons.'
                )}
              </p>
              {thresholdFields.map(([metric, key]) =>
                textField(metric, t(key), 'number')
              )}
            </SettingsForm>
          </Form>
          <TestMailForm />
        </>
      )}
    </SettingsSection>
  )
}

function TestMailForm() {
  const { t } = useTranslation()
  const [recipient, setRecipient] = useState('')
  const [sending, setSending] = useState(false)

  async function send(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (sending) return
    setSending(true)
    try {
      const result = await api.post('/api/option/mail/test', { recipient })
      requireServerSuccess(result.data)
      toast.success(t('Test email submitted. Check the recipient inbox.'))
    } catch (error) {
      handleServerError(error)
    } finally {
      setSending(false)
    }
  }

  return (
    <form className='space-y-3 border-t pt-5' onSubmit={send}>
      <Label htmlFor='test-mail-recipient'>{t('Test email recipient')}</Label>
      <Input
        id='test-mail-recipient'
        type='email'
        required
        value={recipient}
        onChange={(event) => setRecipient(event.target.value)}
      />
      <Button variant='outline' type='submit' disabled={sending}>
        {t('Send test email')}
      </Button>
      <p className='text-muted-foreground text-sm'>
        {t('The test uses the saved mail configuration.')}
      </p>
    </form>
  )
}
