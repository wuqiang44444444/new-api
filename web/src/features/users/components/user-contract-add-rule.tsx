import { Plus } from 'lucide-react'
import { useId } from 'react'
import { useTranslation } from 'react-i18next'

import { MultiSelect } from '@/components/multi-select'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'

import type { CustomerContractChannelGroupOption } from '../types'
import { parseContractDiscount } from './user-contract-utils'

type CustomerContractAddRuleProps = {
  channelGroups: CustomerContractChannelGroupOption[]
  group: string
  models: string[]
  channelIdsByModel: Record<string, string[]>
  discount: string
  onGroupChange: (value: string) => void
  onModelsChange: (values: string[]) => void
  onModelChannelsChange: (model: string, values: string[]) => void
  onDiscountChange: (value: string) => void
  onAdd: () => void
}

export function CustomerContractAddRule(props: CustomerContractAddRuleProps) {
  const { t } = useTranslation()
  const id = useId()
  const selectedGroupModels =
    props.channelGroups.find((group) => group.group === props.group)?.models ||
    []
  // Every model of the group stays selectable: the same public model may be
  // listed on several channels as long as the discount matches.
  const availableModels = selectedGroupModels.map((entry) => entry.model)
  // Rule count = the channels picked for each pending model; models without
  // a channel stay visible as problems instead of counting as zero rules.
  const ruleCount = props.models.reduce(
    (count, model) => count + (props.channelIdsByModel[model]?.length || 0),
    0
  )

  return (
    <section
      aria-labelledby={`${id}-title`}
      className='@container/contract-add min-w-0 rounded-xl border'
    >
      <div className='space-y-1 border-b px-4 py-3'>
        <h3 id={`${id}-title`} className='text-sm font-semibold'>
          {t('Add model rule')}
        </h3>
        <FieldDescription className='text-xs leading-relaxed'>
          {t(
            'One public model may list several channels; every rule of a model must share the same discount.'
          )}
        </FieldDescription>
      </div>
      <div className='space-y-3 p-4'>
        <FieldGroup className='grid items-start gap-3 @2xl/contract-add:grid-cols-[150px_minmax(0,1fr)_140px]'>
          <Field className='min-w-0'>
            <FieldLabel htmlFor={`${id}-group`}>{t('Route group')}</FieldLabel>
            <Combobox
              id={`${id}-group`}
              options={props.channelGroups.map((group) => ({
                value: group.group,
                label: group.group,
              }))}
              value={props.group}
              onValueChange={(value) => {
                props.onGroupChange(value || '')
                props.onModelsChange([])
              }}
              placeholder={t('Select route group')}
              emptyText={t('No available model in this group')}
              openOnFocus
            />
          </Field>
          <Field className='min-w-0'>
            <FieldLabel htmlFor={`${id}-models`}>{t('Model')}</FieldLabel>
            <MultiSelect
              id={`${id}-models`}
              options={availableModels.map((model) => ({
                value: model,
                label: model,
              }))}
              selected={props.models}
              onChange={props.onModelsChange}
              placeholder={t('Search and select models')}
              emptyText={t('No available model in this group')}
              selectAll
              maxVisibleChips={2}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor={`${id}-discount`}>
              {t('Contract discount')}
            </FieldLabel>
            <Input
              id={`${id}-discount`}
              value={props.discount}
              onChange={(event) => props.onDiscountChange(event.target.value)}
              aria-invalid={parseContractDiscount(props.discount) === null}
              placeholder={t('e.g. 0.8, 80%, or 8折')}
            />
          </Field>
        </FieldGroup>
        <FieldDescription className='text-xs leading-relaxed'>
          {t(
            'Select all covers every candidate model in this group, not just the current search results.'
          )}
        </FieldDescription>
        {props.models.length > 0 && (
          <div className='rounded-lg border'>
            <div
              aria-hidden='true'
              className='bg-muted/40 text-muted-foreground hidden gap-4 rounded-t-lg border-b px-3 py-2 text-xs font-medium @lg/contract-add:grid @lg/contract-add:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]'
            >
              <span>{t('Model')}</span>
              <span>{t('Channel')}</span>
            </div>
            <div className='max-h-72 divide-y overflow-y-auto overscroll-contain'>
              {props.models.map((model) => {
                const modelEntry = selectedGroupModels.find(
                  (entry) => entry.model === model
                )
                const candidates = modelEntry?.channels || []
                const selected = props.channelIdsByModel[model] || []
                const staleChannelIds = selected.filter(
                  (id) =>
                    !candidates.some((channel) => String(channel.id) === id)
                )
                let rowProblem = null
                if (!modelEntry) {
                  rowProblem = (
                    <FieldDescription className='text-destructive'>
                      {t(
                        'Model {{model}} is no longer a candidate in this group. Reselect models.',
                        { model }
                      )}
                    </FieldDescription>
                  )
                } else if (candidates.length === 0) {
                  rowProblem = (
                    <FieldDescription>
                      {t('No qualifying channel is available for this model')}
                    </FieldDescription>
                  )
                } else if (selected.length === 0) {
                  rowProblem = (
                    <FieldDescription>
                      {t('Select a channel for model {{model}}', { model })}
                    </FieldDescription>
                  )
                }
                return (
                  <div
                    key={model}
                    className='grid min-w-0 gap-2 p-3 @lg/contract-add:grid-cols-[minmax(0,1fr)_minmax(0,2fr)] @lg/contract-add:items-start @lg/contract-add:gap-4'
                  >
                    <div className='min-w-0 @lg/contract-add:pt-1.5'>
                      <p className='font-mono text-sm font-medium break-all'>
                        {model}
                      </p>
                    </div>
                    <Field className='min-w-0'>
                      <FieldLabel
                        className='sr-only'
                        htmlFor={`${id}-channels-${model}`}
                      >
                        {t('Channels for model {{model}}', { model })}
                      </FieldLabel>
                      <MultiSelect
                        id={`${id}-channels-${model}`}
                        className='bg-background min-w-0 [&_[data-slot=combobox-chip]]:max-w-[calc(100%-5rem)]'
                        options={candidates.map((channel) => ({
                          value: String(channel.id),
                          label: channel.name,
                        }))}
                        selected={selected}
                        onChange={(values) =>
                          props.onModelChannelsChange(model, values)
                        }
                        placeholder={t('Select channels')}
                        inputAriaLabel={t('Channels for model {{model}}', {
                          model,
                        })}
                        emptyText={t('No available model in this group')}
                        selectAll
                        renderSelectedSummary={
                          selected.length > 1
                            ? (values) =>
                                t('{{count}} selected', {
                                  count: values.length,
                                })
                            : undefined
                        }
                      />
                      {rowProblem}
                      {modelEntry && staleChannelIds.length > 0 && (
                        <FieldDescription className='text-destructive'>
                          {t(
                            'Selected channels of {{model}} are no longer available. Reselect its channels.',
                            { model }
                          )}
                        </FieldDescription>
                      )}
                    </Field>
                  </div>
                )
              })}
            </div>
          </div>
        )}
      </div>
      <div className='bg-muted/20 flex flex-wrap items-center justify-between gap-3 rounded-b-xl border-t px-4 py-3'>
        <p role='status' className='text-muted-foreground text-xs tabular-nums'>
          {t('Will add {{modelCount}} models and {{ruleCount}} rules', {
            modelCount: props.models.length,
            ruleCount,
          })}
        </p>
        <Button
          type='button'
          onClick={props.onAdd}
          disabled={props.models.length === 0}
          className='ml-auto'
        >
          <Plus aria-hidden='true' /> {t('Add')}
        </Button>
      </div>
    </section>
  )
}
