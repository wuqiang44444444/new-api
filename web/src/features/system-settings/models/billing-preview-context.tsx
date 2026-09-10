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
import { useId } from 'react'
import { useTranslation } from 'react-i18next'

import { Checkbox } from '@/components/ui/checkbox'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'

export function BillingPreviewContext(props: {
  time: string
  onTimeChange: (value: string) => void
  needsContext: boolean
  json: string
  onJSONChange: (value: string) => void
  allowMissing: boolean
  onAllowMissingChange: (value: boolean) => void
}) {
  const { t } = useTranslation()
  const timeId = useId()
  const contextId = useId()
  const missingId = useId()
  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone
  return (
    <div className='space-y-3'>
      <Field>
        <FieldLabel htmlFor={timeId}>
          {t('Simulation time')} ({timezone})
        </FieldLabel>
        <Input
          id={timeId}
          type='datetime-local'
          step='1'
          value={props.time}
          onChange={(event) => props.onTimeChange(event.target.value)}
        />
        <FieldDescription>
          {t(
            'Leave empty to use server time. Expression timezones still apply.'
          )}
        </FieldDescription>
      </Field>
      {props.needsContext && (
        <>
          <Field>
            <FieldLabel htmlFor={contextId}>
              {t('Trial request context')}
            </FieldLabel>
            <Textarea
              id={contextId}
              value={props.json}
              onChange={(event) => props.onJSONChange(event.target.value)}
              placeholder='{"body": {}, "headers": {}}'
            />
            <FieldDescription>
              {t('Use synthetic values only. This context is not saved.')}
            </FieldDescription>
          </Field>
          <Field orientation='horizontal'>
            <Checkbox
              id={missingId}
              checked={props.allowMissing}
              onCheckedChange={(value) =>
                props.onAllowMissingChange(value === true)
              }
            />
            <FieldLabel htmlFor={missingId}>
              {t('Simulate missing request values')}
            </FieldLabel>
          </Field>
        </>
      )}
    </div>
  )
}
