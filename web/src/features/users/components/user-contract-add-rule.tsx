import { Plus } from 'lucide-react'
import type { Dispatch, SetStateAction } from 'react'
import { useTranslation } from 'react-i18next'

import { MultiSelect } from '@/components/multi-select'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'

import type {
  CustomerContractGroupOption,
  CustomerContractRule,
} from '../types'
import { parseContractDiscount } from './user-contract-utils'

type CustomerContractAddRuleProps = {
  options: CustomerContractGroupOption[]
  rules: CustomerContractRule[]
  group: string
  setGroup: Dispatch<SetStateAction<string>>
  models: string[]
  setModels: Dispatch<SetStateAction<string[]>>
  discount: string
  setDiscount: Dispatch<SetStateAction<string>>
  onAdd: () => void
}

export function CustomerContractAddRule({
  options,
  rules,
  group,
  setGroup,
  models,
  setModels,
  discount,
  setDiscount,
  onAdd,
}: CustomerContractAddRuleProps) {
  const { t } = useTranslation()
  const selectedGroupModels =
    options.find((option) => option.group === group)?.models || []
  const availableModels = selectedGroupModels.filter(
    (model) =>
      !rules.some((rule) => rule.model.toLowerCase() === model.toLowerCase())
  )

  return (
    <Field>
      <FieldLabel>{t('Add model rule')}</FieldLabel>
      <div className='grid gap-2 md:grid-cols-[200px_minmax(240px,1fr)_150px_auto] md:items-end'>
        <Field>
          <FieldLabel>{t('Route group')}</FieldLabel>
          <Combobox
            options={options.map((option) => ({
              value: option.group,
              label: option.group,
            }))}
            value={group}
            onValueChange={(value) => {
              setGroup(value || '')
              setModels([])
            }}
            placeholder={t('Select route group')}
            emptyText={t('No available model in this group')}
            openOnFocus
          />
        </Field>
        <Field>
          <FieldLabel>{t('Models')}</FieldLabel>
          <MultiSelect
            options={availableModels.map((model) => ({
              value: model,
              label: model,
            }))}
            selected={models}
            onChange={setModels}
            placeholder={t('Search and select a model')}
            emptyText={t('No available model in this group')}
            maxVisibleChips={3}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor='contract-batch-discount'>
            {t('Contract discount')}
          </FieldLabel>
          <Input
            id='contract-batch-discount'
            value={discount}
            onChange={(event) => setDiscount(event.target.value)}
            aria-invalid={parseContractDiscount(discount) === null}
            placeholder={t('e.g. 0.8, 80%, or 8折')}
          />
        </Field>
        <Button type='button' onClick={onAdd} disabled={models.length === 0}>
          <Plus /> {t('Add')}
        </Button>
      </div>
      <FieldDescription>
        {t('Each public model can bind to exactly one route group.')}
      </FieldDescription>
    </Field>
  )
}
