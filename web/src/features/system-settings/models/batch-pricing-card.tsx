import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Field, FieldLabel } from '@/components/ui/field'
import { Textarea } from '@/components/ui/textarea'

import { useUpdateOption } from '../hooks/use-update-option'

export function BatchPricingCard({ value }: { value: string }) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState(value)
  const mutation = useUpdateOption()
  let valid = false
  try {
    const parsed: unknown = JSON.parse(draft)
    valid =
      parsed !== null &&
      typeof parsed === 'object' &&
      !Array.isArray(parsed) &&
      Object.entries(parsed).every(
        ([model, expr]) =>
          model.trim() && typeof expr === 'string' && expr.trim()
      )
  } catch {
    /* Invalid drafts remain editable. */
  }
  return (
    <div className='space-y-4'>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Batch prices use token and time expressions. Prices are evaluated per request and are separate from synchronous prices.'
        )}
      </p>
      <Field>
        <FieldLabel htmlFor='batch-prices'>
          {t('Batch billing expressions')}
        </FieldLabel>
        <Textarea
          id='batch-prices'
          className='min-h-64 font-mono'
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          aria-invalid={!valid}
        />
      </Field>
      <Button
        disabled={!valid || mutation.isPending || draft === value}
        onClick={() =>
          mutation.mutate({
            key: 'batch_billing_setting.batch_billing_expr',
            value: draft,
          })
        }
      >
        {t('Save')}
      </Button>
    </div>
  )
}
