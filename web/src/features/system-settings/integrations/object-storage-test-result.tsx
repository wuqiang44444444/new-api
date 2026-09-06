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
import { CheckCircle2, TriangleAlert, XCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'

import type { ObjectStorageTestResult } from './object-storage-api'

const STEP_LABEL_KEYS: Record<string, string> = {
  validate: 'Validate configuration',
  build: 'Build storage client',
  upload: 'Upload test object',
  head: 'Verify object properties',
  get: 'Authenticated read',
  signed_get: 'Signed URL read',
  cleanup: 'Clean up test object',
}

export function ObjectStorageTestResultAlert({
  result,
  title,
}: {
  result: ObjectStorageTestResult
  title?: string
}) {
  const { t } = useTranslation()

  return (
    <Alert
      variant={result.success ? 'default' : 'destructive'}
      className='flex flex-col items-start gap-2'
    >
      <div className='flex items-center gap-2'>
        {result.success ? (
          <CheckCircle2 className='size-4 text-green-600' />
        ) : (
          <XCircle className='size-4' />
        )}
        <div>
          <AlertTitle>
            {title ?? (result.success ? t('Connection successful') : t('Connection failed'))}
          </AlertTitle>
          {result.message ? (
            <AlertDescription>{t(result.message)}</AlertDescription>
          ) : null}
        </div>
      </div>
      {result.steps && result.steps.length > 0 ? (
        <ul className='space-y-1 text-sm'>
          {result.steps.map((step) => (
            <li key={step.name} className='flex items-center gap-2'>
              {step.success ? (
                <CheckCircle2 className='size-3.5 shrink-0 text-green-600' />
              ) : (
                <XCircle className='size-3.5 shrink-0 text-red-500' />
              )}
              <span>
                {t(STEP_LABEL_KEYS[step.name] ?? step.name)}
                {!step.success && step.detail ? `: ${step.detail}` : ''}
              </span>
            </li>
          ))}
        </ul>
      ) : null}
      {result.cleanup_failed ? (
        <div className='flex items-center gap-2 text-sm text-amber-600'>
          <TriangleAlert className='size-3.5 shrink-0' />
          <span>
            {t(
              'The test object could not be cleaned up automatically; please remove it manually in the storage container.'
            )}
          </span>
        </div>
      ) : null}
    </Alert>
  )
}
