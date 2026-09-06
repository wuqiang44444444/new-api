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
import { Loader2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { formatDateTimeStr } from '@/lib/format'

import {
  getObjectStorageSetting,
  importObjectStorageEnvConfig,
  previewObjectStorageEnvImport,
  saveObjectStorageSetting,
  testObjectStorageConnection,
  type ObjectStorageBackend,
  type ObjectStorageEnvImportPreview,
  type ObjectStorageSettingRequest,
  type ObjectStorageTestResult,
} from './object-storage-api'
import { ObjectStorageTestResultAlert } from './object-storage-test-result'
import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'

const BACKEND_LABELS: Record<ObjectStorageBackend, string> = {
  upstream: 'Disabled (no object storage)',
  s3: 'S3 compatible',
  azure_blob: 'Azure Blob',
}

const BACKEND_OPTIONS: Array<{ value: ObjectStorageBackend; label: string }> = (
  Object.keys(BACKEND_LABELS) as Array<ObjectStorageBackend>
).map((value) => ({ value, label: BACKEND_LABELS[value] }))

const createSchema = (t: (key: string) => string) =>
  z
    .object({
      backend: z.enum(['upstream', 's3', 'azure_blob']),
      endpoint: z.string(),
      bucket: z.string(),
      prefix: z.string(),
      region: z.string(),
      account_name: z.string(),
      credential: z.string(),
      inputMode: z.enum(['connection_string', 'manual']),
      connectionString: z.string(),
    })
    .superRefine((values, ctx) => {
      if (values.backend === 'upstream') return
      if (!values.bucket.trim()) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['bucket'],
          message: t('Bucket or container is required'),
        })
      }
      if (values.inputMode === 'manual' || values.backend === 's3') {
        if (!values.endpoint.trim()) {
          ctx.addIssue({
            code: z.ZodIssueCode.custom,
            path: ['endpoint'],
            message: t('Endpoint is required'),
          })
        }
        if (!values.account_name.trim()) {
          ctx.addIssue({
            code: z.ZodIssueCode.custom,
            path: ['account_name'],
            message: t('Account name is required'),
          })
        }
      }
      if (values.backend === 's3' && !values.region.trim()) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['region'],
          message: t('Region is required'),
        })
      }
    })

type ObjectStorageFormValues = z.input<ReturnType<typeof createSchema>>

export function ObjectStorageSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const settingsQuery = useQuery({
    queryKey: ['object-storage-setting'],
    queryFn: getObjectStorageSetting,
  })
  const view = settingsQuery.data?.data

  const [testResult, setTestResult] = useState<ObjectStorageTestResult | null>(
    null
  )
  const [testing, setTesting] = useState(false)
  const [saving, setSaving] = useState(false)
  const [envPreview, setEnvPreview] =
    useState<ObjectStorageEnvImportPreview | null>(null)
  const [importing, setImporting] = useState(false)

  const schema = useMemo(() => createSchema(t), [t])

  const form = useForm<ObjectStorageFormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      backend: 'upstream',
      endpoint: '',
      bucket: '',
      prefix: '',
      region: '',
      account_name: '',
      credential: '',
      inputMode: 'connection_string',
      connectionString: '',
    },
  })

  // 预填账号和连接位置，只有密钥不回显。已保存 Azure 配置以手动字段继续编辑。
  const formDefaults = useMemo(
    () => ({
      backend: (view?.backend || 'upstream') as ObjectStorageBackend,
      endpoint: view?.endpoint ?? '',
      bucket: view?.bucket ?? '',
      prefix: view?.prefix ?? '',
      region: view?.region ?? '',
      account_name: view?.account_name ?? '',
      credential: '',
      inputMode: (view?.credential_configured ? 'manual' : 'connection_string') as 'connection_string' | 'manual',
      connectionString: '',
    }),
    [view]
  )
  useResetForm(form, formDefaults)

  const backend = form.watch('backend')
  const effectiveInputMode = form.watch('inputMode') ?? 'connection_string'


  function buildRequest(values: ObjectStorageFormValues): ObjectStorageSettingRequest {
    return {
      backend: values.backend,
      endpoint: (values.endpoint ?? '').trim(),
      bucket: (values.bucket ?? '').trim(),
      prefix: (values.prefix ?? '').trim(),
      region: (values.region ?? '').trim(),
      account_name: (values.account_name ?? '').trim(),
      credential: values.credential ?? '',
      input_mode: values.inputMode,
      connection_string: values.connectionString ?? '',
    }
  }

  const handleTest = async () => {
    const values = form.getValues()
    const parsed = schema.safeParse(values)
    if (!parsed.success) {
      toast.error(t('Please complete the configuration before testing'))
      return
    }
    setTesting(true)
    setTestResult(null)
    try {
      const res = await testObjectStorageConnection(buildRequest(values))
      setTestResult(res.data ?? { success: false, cleanup_failed: false, message: res.message })
      if (res.success) {
        toast.success(t(res.message || 'Connection successful'))
      }
    } finally {
      setTesting(false)
    }
  }

  const handleSave = async (values: ObjectStorageFormValues) => {
    setSaving(true)
    setTestResult(null)
    try {
      const res = await saveObjectStorageSetting(buildRequest(values))
      if (res.success) {
        toast.success(t(res.message || 'Object storage settings saved'))
        if (res.data) {
          setTestResult(res.data)
        }
        await queryClient.invalidateQueries({
          queryKey: ['object-storage-setting'],
        })
        form.setValue('credential', '')
 form.setValue('connectionString', '')
      } else {
        if (res.data) {
          setTestResult(res.data)
        }
        toast.error(t(res.message || 'Save failed'))
      }
    } finally {
      setSaving(false)
    }
  }

  const handleEnvPreview = async () => {
    const res = await previewObjectStorageEnvImport()
    if (res.data) {
      setEnvPreview(res.data)
    } else {
      toast.error(t(res.message || 'No importable configuration'))
    }
  }

  const handleEnvImport = async () => {
    setImporting(true)
    try {
      const res = await importObjectStorageEnvConfig()
      if (res.success) {
        toast.success(t(res.message || 'Configuration imported'))
        setEnvPreview(null)
        await queryClient.invalidateQueries({
          queryKey: ['object-storage-setting'],
        })
      } else {
        if (res.data) {
          setTestResult(res.data)
        }
        toast.error(t(res.message || 'Import failed'))
      }
    } finally {
      setImporting(false)
    }
  }

  const lastTestAt = view?.last_test_at
    ? formatDateTimeStr(new Date(view.last_test_at * 1000))
    : ''
  function verificationStatusLabel(status: string | undefined): string {
    if (status === 'passed') return t('Last test passed')
    if (status === 'failed') return t('Last test failed')
    return t('Not verified')
  }
  const verificationLabel = verificationStatusLabel(view?.last_test_status)

  return (
    <SettingsSection title={t('Object Storage')}>
      <Form {...form}>
        <SettingsForm
          onSubmit={form.handleSubmit(handleSave)}
          autoComplete='off'
        >
          <SettingsPageFormActions
            onSave={form.handleSubmit(handleSave)}
            isSaving={saving}
            saveLabel='Save object storage settings'
          />

          {view?.active ? (
 <p className='text-sm text-muted-foreground'>
 {t('Storage location changes require offline maintenance and data migration; only credentials can be updated here.')}
 </p>
 ) : null}

 {view ? (
            <Alert variant='default'>
              <AlertDescription>
                <div className='space-y-1 text-sm'>
                  <div>
                    {t('Current status')}:{' '}
                    {view.active
                      ? t('Enabled')
                      : t('Disabled')}
                    {view.backend
                      ? ` · ${t(BACKEND_LABELS[view.backend] ?? view.backend)}`
                      : ''}
                  </div>
                  <div>
                    {t('Credential status')}:{' '}
                    {view.credential_configured
                      ? t('Configured')
                      : t('Not configured')}
                    {view.account_name_masked
                      ? ` · ${view.account_name_masked}`
                      : ''}
                  </div>
                  {view.endpoint ? (
                    <div>
                      {t('Endpoint')}: {view.endpoint}
                      {view.bucket ? ` · ${view.bucket}` : ''}
                      {view.prefix ? ` · ${view.prefix}` : ''}
                    </div>
                  ) : null}
                  <div>
                    {t('Verification status')}:{' '}
                    {verificationLabel}
                    {lastTestAt ? ` · ${lastTestAt}` : ''}
                  </div>
                </div>
              </AlertDescription>
            </Alert>
          ) : null}

          {view?.env_import_available ? (
            <Alert variant='default'>
              <AlertTitle>{t('Legacy environment configuration detected')}</AlertTitle>
              <AlertDescription>
                <div className='space-y-2'>
                  <p>
                    {t(
                      'An S3-compatible configuration was found in startup environment variables. Import it once into the database; afterwards the database is the only configuration source and the environment variables no longer take effect.'
                    )}
                  </p>
                  {envPreview ? (
                    <ul className='list-disc space-y-1 pl-5 text-sm'>
                      <li>
                        {t('Type')}: {t(BACKEND_LABELS[envPreview.backend as ObjectStorageBackend] ?? envPreview.backend)}
                      </li>
                      <li>{t('Endpoint')}: {envPreview.endpoint}</li>
                      <li>{t('Bucket / Container')}: {envPreview.bucket}</li>
                      <li>{t('Region')}: {envPreview.region || '-'}</li>
                      <li>{t('Object prefix')}: {envPreview.prefix || '-'}</li>
                      <li>{t('Account name')}: {envPreview.account_name_masked}</li>
                    </ul>
                  ) : null}
                  <div className='flex gap-2'>
                    <Button
                      type='button'
                      variant='outline'
                      onClick={handleEnvPreview}
                    >
                      {t('Preview import')}
                    </Button>
                    <Button
                      type='button'
                      variant='secondary'
                      onClick={handleEnvImport}
                      disabled={importing}
                    >
                      {importing ? (
                        <Loader2 className='me-2 size-4 animate-spin' />
                      ) : null}
                      {t('Verify and import')}
                    </Button>
                  </div>
                </div>
              </AlertDescription>
            </Alert>
          ) : null}

          <FormField
            control={form.control}
            name='backend'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Storage type')}</FormLabel>
                <Select
                  value={field.value}
                  onValueChange={(value) => field.onChange(value)}
                >
                  <FormControl>
                    <SelectTrigger className='w-full'>
                      <SelectValue />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent>
                    {BACKEND_OPTIONS.map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        {t(option.label)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <FormDescription>
                  {t(
                    'Disabled keeps existing upstream proxying behavior; saved image task artifacts rely on object storage.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          {backend !== 'upstream' ? (
            <>
              <FormField
                control={form.control}
                name='bucket'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {backend === 'azure_blob'
                        ? t('Container')
                        : t('Bucket')}
                    </FormLabel>
                    <FormControl>
                      <Input placeholder='task-artifacts' {...field} />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'The container or bucket must already exist; it will not be created automatically.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='prefix'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Object prefix')}</FormLabel>
                    <FormControl>
                      <Input placeholder='artifacts/' {...field} />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Optional. Applied to uploads and object references; empty is valid and will not be duplicated.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {backend === 'azure_blob' ? (
                <>
                  <FormItem>
                    <FormLabel>{t('Azure connection information')}</FormLabel>
                    <div className='flex gap-2'>
                      <Button
                        type='button'
                        size='sm'
                        variant={
                          effectiveInputMode === 'connection_string'
                            ? 'default'
                            : 'outline'
                        }
                        onClick={() => form.setValue('inputMode', 'connection_string')}
                      >
                        {t('Connection string')}
                      </Button>
                      <Button
                        type='button'
                        size='sm'
                        variant={
                          effectiveInputMode === 'manual' ? 'default' : 'outline'
                        }
                        onClick={() => form.setValue('inputMode', 'manual')}
                      >
                        {t('Manual account')}
                      </Button>
                    </div>
                    <FormDescription>
                      {t(
                        'The connection string is only an input format and is parsed into normalized fields; the raw value is never saved.'
                      )}
                    </FormDescription>
                  </FormItem>

                  {effectiveInputMode === 'connection_string' ? (
                    <FormField
                      control={form.control}
                      name='connectionString'
                      render={({ field }) => (
                        <FormItem>
                          <FormControl>
                            <Input
                              type='password'
                              placeholder='DefaultEndpointsProtocol=https;AccountName=...;AccountKey=...;EndpointSuffix=core.windows.net'
                              autoComplete='new-password'
                              {...field}
                            />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  ) : (
                    <>
                      <FormField
                        control={form.control}
                        name='endpoint'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('Endpoint')}</FormLabel>
                            <FormControl>
                              <Input
                                type='url'
                                placeholder='https://<account>.blob.core.windows.net'
                                {...field}
                              />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      <FormField
                        control={form.control}
                        name='account_name'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('Account name')}</FormLabel>
                            <FormControl>
                              <Input autoComplete='off' {...field} />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                    </>
                  )}

                  <FormField
                    control={form.control}
                    name='credential'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Account key')}</FormLabel>
                        <FormControl>
                          <Input
                            type='password'
                            placeholder={
                              view?.credential_configured
                                ? t('Configured; leave blank to keep the existing key')
                                : t('Enter new key to update')
                            }
                            autoComplete='new-password'
                            {...field}
                          />
                        </FormControl>
                        <FormDescription>
                          {t(
                            'Only SharedKey credentials are supported. The key is stored encrypted and is never displayed again.'
                          )}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </>
              ) : (
                <>
                  <FormField
                    control={form.control}
                    name='endpoint'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Endpoint')}</FormLabel>
                        <FormControl>
                          <Input
                            type='url'
                            placeholder='https://s3.example.com'
                            {...field}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={form.control}
                    name='region'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Region')}</FormLabel>
                        <FormControl>
                          <Input placeholder='us-east-1' {...field} />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={form.control}
                    name='account_name'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Access key ID')}</FormLabel>
                        <FormControl>
                          <Input autoComplete='off' {...field} />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={form.control}
                    name='credential'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Secret access key')}</FormLabel>
                        <FormControl>
                          <Input
                            type='password'
                            placeholder={
                              view?.credential_configured
                                ? t('Configured; leave blank to keep the existing key')
                                : t('Enter new key to update')
                            }
                            autoComplete='new-password'
                            {...field}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </>
              )}

              <div className='flex items-center gap-2'>
                <Button
                  type='button'
                  variant='secondary'
                  onClick={handleTest}
                  disabled={testing || saving}
                >
                  {testing ? (
                    <Loader2 className='me-2 size-4 animate-spin' />
                  ) : null}
                  {t('Test Connection')}
                </Button>
              </div>

              {testResult ? (
                <ObjectStorageTestResultAlert result={testResult} />
              ) : null}

            </>
          ) : null}
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}

