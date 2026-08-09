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
import { useQuery } from '@tanstack/react-query'
import { useEffect, useMemo, useRef, useState } from 'react'
import { type Control, useFormContext, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
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

import { getLinkImplementations, getLinkModelPublications } from '../../api'
import type { ChannelFormValues } from '../../lib/channel-form'
import {
  deriveLinkPublicationPreviews,
  EMPTY_LINK_ACCESS_PLAN_PROJECTION,
  linkAccessPlanAutofill,
  linkAccessPlanLabel,
  linkAccessPlanOptionValue,
  linkAccessPlanProviderModelDefaults,
  linkAccessPlansForChannelType,
  type LinkAccessPlanProjection,
} from '../../lib/link-access-plan'
import type { LinkModelPublication } from '../../types'
import { LinkPublicationRebindDialog } from '../dialogs/link-publication-rebind-dialog'

const NO_LINK_IMPLEMENTATION = '__none__'
const linkAccessPlanSelectContentClass = 'w-[460px] max-w-[calc(100vw-2rem)]'
const linkAccessPlanSelectItemClass =
  'items-start py-2 [&_[data-slot=select-item-text]]:min-w-0 [&_[data-slot=select-item-text]]:shrink [&_[data-slot=select-item-text]]:whitespace-normal'

interface LinkImplementationFieldProps {
  control: Control<ChannelFormValues>
  channelType: number
  canRebind: boolean
}

export function LinkImplementationField(props: LinkImplementationFieldProps) {
  const { t } = useTranslation()
  const form = useFormContext<ChannelFormValues>()
  const models = useWatch({ control: props.control, name: 'models' }) || ''
  const modelMapping =
    useWatch({ control: props.control, name: 'model_mapping' }) || ''
  const selectedID =
    useWatch({ control: props.control, name: 'link_implementation_id' }) || ''
  const selectedVersion =
    useWatch({
      control: props.control,
      name: 'link_implementation_version',
    }) || ''
  const ordinaryProjectionRef = useRef<LinkAccessPlanProjection | null>(null)
  const [rebindTarget, setRebindTarget] = useState<{
    publication: LinkModelPublication
    linkSKU: string
  } | null>(null)
  const { data, isLoading } = useQuery({
    queryKey: ['link_implementations'],
    queryFn: getLinkImplementations,
  })
  const { data: publicationData } = useQuery({
    queryKey: ['link_model_publications'],
    queryFn: getLinkModelPublications,
  })
  const implementations = useMemo(
    () => linkAccessPlansForChannelType(data?.data || [], props.channelType),
    [data?.data, props.channelType]
  )
  const selectedImplementation = useMemo(
    () =>
      implementations.find(
        (implementation) =>
          implementation.id === selectedID &&
          implementation.version === selectedVersion
      ),
    [implementations, selectedID, selectedVersion]
  )
  const selectedOptionValue = selectedImplementation
    ? linkAccessPlanOptionValue(selectedImplementation)
    : NO_LINK_IMPLEMENTATION
  const previews = useMemo(
    () =>
      selectedImplementation
        ? deriveLinkPublicationPreviews(
            selectedImplementation,
            models,
            modelMapping
          )
        : [],
    [modelMapping, models, selectedImplementation]
  )
  const projection = useMemo(
    () =>
      selectedImplementation
        ? linkAccessPlanAutofill(selectedImplementation, previews)
        : null,
    [previews, selectedImplementation]
  )

  useEffect(() => {
    if (!projection) return
    form.setValue(
      'video_upstream_profile',
      projection.video_upstream_profile as ChannelFormValues['video_upstream_profile']
    )
    form.setValue(
      'asset_upstream_profile',
      projection.asset_upstream_profile as ChannelFormValues['asset_upstream_profile']
    )
    form.setValue(
      'video_upstream_create_path',
      projection.video_upstream_create_path
    )
    form.setValue(
      'video_upstream_query_path_template',
      projection.video_upstream_query_path_template
    )
    form.setValue(
      'asset_min_url_ttl_seconds',
      projection.asset_min_url_ttl_seconds
    )
    form.setValue('advanced_custom', projection.advanced_custom)
  }, [form, projection])

  return (
    <div className='space-y-3'>
      <FormField
        control={props.control}
        name='link_implementation_id'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('Link Access Plan')}</FormLabel>
            <Select
              disabled={isLoading}
              items={[
                {
                  value: NO_LINK_IMPLEMENTATION,
                  label: t('No Link access plan'),
                },
                ...implementations.map((implementation) => ({
                  value: linkAccessPlanOptionValue(implementation),
                  label: linkAccessPlanLabel(implementation),
                })),
              ]}
              value={selectedOptionValue}
              onValueChange={(value) => {
                if (value === NO_LINK_IMPLEMENTATION) {
                  field.onChange('')
                  form.setValue('link_implementation_version', '', {
                    shouldDirty: true,
                    shouldValidate: true,
                  })
                  const ordinaryProjection =
                    ordinaryProjectionRef.current ||
                    EMPTY_LINK_ACCESS_PLAN_PROJECTION
                  form.setValue(
                    'video_upstream_profile',
                    ordinaryProjection.video_upstream_profile as ChannelFormValues['video_upstream_profile']
                  )
                  form.setValue(
                    'asset_upstream_profile',
                    ordinaryProjection.asset_upstream_profile as ChannelFormValues['asset_upstream_profile']
                  )
                  form.setValue(
                    'video_upstream_create_path',
                    ordinaryProjection.video_upstream_create_path
                  )
                  form.setValue(
                    'video_upstream_query_path_template',
                    ordinaryProjection.video_upstream_query_path_template
                  )
                  form.setValue(
                    'asset_min_url_ttl_seconds',
                    ordinaryProjection.asset_min_url_ttl_seconds
                  )
                  form.setValue(
                    'advanced_custom',
                    ordinaryProjection.advanced_custom
                  )
                  ordinaryProjectionRef.current = null
                  return
                }
                const implementation = implementations.find(
                  (candidate) => linkAccessPlanOptionValue(candidate) === value
                )
                if (!implementation) return
                if (!selectedID) {
                  ordinaryProjectionRef.current = {
                    video_upstream_profile:
                      form.getValues('video_upstream_profile') || 'official',
                    asset_upstream_profile:
                      form.getValues('asset_upstream_profile') || 'none',
                    video_upstream_create_path:
                      form.getValues('video_upstream_create_path') || '',
                    video_upstream_query_path_template:
                      form.getValues('video_upstream_query_path_template') ||
                      '',
                    asset_min_url_ttl_seconds:
                      form.getValues('asset_min_url_ttl_seconds') || 0,
                    advanced_custom: form.getValues('advanced_custom') || '',
                  }
                }
                field.onChange(implementation.id)
                form.setValue(
                  'link_implementation_version',
                  implementation.version,
                  { shouldDirty: true, shouldValidate: true }
                )
                const currentModels = form.getValues('models').trim()
                const currentModelMapping = (
                  form.getValues('model_mapping') || ''
                ).trim()
                if (!currentModels && !currentModelMapping) {
                  const providerModels =
                    linkAccessPlanProviderModelDefaults(implementation)
                  if (providerModels.length > 0) {
                    form.setValue('models', providerModels.join(','), {
                      shouldDirty: true,
                      shouldValidate: true,
                    })
                  }
                }
              }}
            >
              <FormControl>
                <SelectTrigger>
                  <SelectValue placeholder={t('Select Link access plan')} />
                </SelectTrigger>
              </FormControl>
              <SelectContent
                alignItemWithTrigger={false}
                className={linkAccessPlanSelectContentClass}
              >
                <SelectGroup>
                  <SelectItem value={NO_LINK_IMPLEMENTATION}>
                    {t('No Link access plan')}
                  </SelectItem>
                  {implementations.map((implementation) => (
                    <SelectItem
                      key={`${implementation.id}/${implementation.version}`}
                      value={linkAccessPlanOptionValue(implementation)}
                      className={linkAccessPlanSelectItemClass}
                    >
                      <span className='min-w-0 leading-snug break-words whitespace-normal'>
                        {linkAccessPlanLabel(implementation)}
                      </span>
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            <FormDescription>
              {field.value
                ? t(
                    'This plan projects the customer models and ordinary model mapping into one immutable Link contract. Protocol fields are filled and locked by the plan.',
                    { id: field.value, version: selectedVersion }
                  )
                : t(
                    'Without a plan, this channel keeps ordinary routing behavior.'
                  )}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
      {selectedImplementation && previews.length > 0 && (
        <Alert>
          <AlertTitle>{linkAccessPlanLabel(selectedImplementation)}</AlertTitle>
          <AlertDescription className='space-y-2 text-xs'>
            {previews.map((preview) => {
              const publication = publicationData?.data.find(
                (candidate) =>
                  candidate.contract_namespace === 'link' &&
                  candidate.route_family === preview.routeFamily &&
                  candidate.customer_model === preview.customerModel
              )
              let publicationStatus = 'Unavailable'
              if (publication?.routing_conflict) {
                publicationStatus = 'Conflict'
              } else if (publication?.currently_fulfillable) {
                publicationStatus = 'Available'
              }
              return (
                <div key={preview.customerModel} className='font-mono'>
                  {preview.customerModel} → {preview.providerModel || '—'}
                  {preview.error ? ` · ${t(preview.error)}` : ''}
                  {publication
                    ? ` · ${t('Published')} · ${t(publicationStatus)}`
                    : ` · ${t('will be published when saved')}`}
                  {publication &&
                    preview.linkSKU &&
                    publication.link_sku !== preview.linkSKU && (
                      <span className='text-destructive inline-flex items-center gap-1'>
                        {' '}
                        · {t('Conflict')}
                        <Button
                          type='button'
                          variant='destructive'
                          size='xs'
                          disabled={!props.canRebind}
                          title={
                            props.canRebind
                              ? t('Rebind Link publication')
                              : t(
                                  'Sensitive channel write permission is required.'
                                )
                          }
                          onClick={() =>
                            setRebindTarget({
                              publication,
                              linkSKU: preview.linkSKU || '',
                            })
                          }
                        >
                          {t('Rebind')}
                        </Button>
                      </span>
                    )}
                </div>
              )
            })}
          </AlertDescription>
        </Alert>
      )}
      <LinkPublicationRebindDialog
        open={rebindTarget !== null}
        onOpenChange={(open) => {
          if (!open) setRebindTarget(null)
        }}
        publication={rebindTarget?.publication || null}
        proposedSKU={rebindTarget?.linkSKU || ''}
      />
    </div>
  )
}
