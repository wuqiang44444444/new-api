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
import { useMemo, useState } from 'react'
import { type Control, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'

import { getLinkImplementations, getLinkModelPublications } from '../../api'
import type { ChannelFormValues } from '../../lib/channel-form'
import { deriveLinkPublicationPreviews } from '../../lib/link-access-plan'
import type { LinkModelPublication } from '../../types'
import { LinkPublicationRebindDialog } from '../dialogs/link-publication-rebind-dialog'

type LinkPublicationConflictFieldProps = {
  control: Control<ChannelFormValues>
  canRebind: boolean
}

export function LinkPublicationConflictField(
  props: LinkPublicationConflictFieldProps
) {
  const { t } = useTranslation()
  const models = useWatch({ control: props.control, name: 'models' }) || ''
  const modelMapping =
    useWatch({ control: props.control, name: 'model_mapping' }) || ''
  const implementationID =
    useWatch({ control: props.control, name: 'link_implementation_id' }) || ''
  const implementationVersion =
    useWatch({
      control: props.control,
      name: 'link_implementation_version',
    }) || ''
  const hasLinkPlan = Boolean(implementationID && implementationVersion)
  const [rebindTarget, setRebindTarget] = useState<{
    publication: LinkModelPublication
    linkSKU: string
  } | null>(null)
  const { data: implementationData } = useQuery({
    queryKey: ['link_implementations'],
    queryFn: getLinkImplementations,
    enabled: hasLinkPlan,
  })
  const { data: publicationData } = useQuery({
    queryKey: ['link_model_publications'],
    queryFn: getLinkModelPublications,
    enabled: hasLinkPlan,
  })
  const implementation = useMemo(
    () =>
      implementationData?.data.find(
        (candidate) =>
          candidate.id === implementationID &&
          candidate.version === implementationVersion
      ),
    [implementationData?.data, implementationID, implementationVersion]
  )
  const conflicts = useMemo(() => {
    if (!implementation) return []

    return deriveLinkPublicationPreviews(
      implementation,
      models,
      modelMapping
    ).flatMap((preview) => {
      if (!preview.linkSKU || !preview.routeFamily) return []
      const publication = publicationData?.data.find(
        (candidate) =>
          candidate.contract_namespace === 'link' &&
          candidate.route_family === preview.routeFamily &&
          candidate.customer_model === preview.customerModel
      )
      if (!publication || publication.link_sku === preview.linkSKU) return []

      return [{ preview, publication }]
    })
  }, [implementation, modelMapping, models, publicationData?.data])

  if (conflicts.length === 0) return null

  return (
    <>
      <Alert variant='destructive'>
        <AlertTitle>{t('Conflict')}</AlertTitle>
        <AlertDescription>
          <div className='flex flex-col gap-2'>
            {conflicts.map(({ preview, publication }) => (
              <div
                key={`${preview.routeFamily}/${preview.customerModel}`}
                className='flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'
              >
                <span className='font-mono text-xs break-all'>
                  {preview.customerModel} → {preview.providerModel}
                </span>
                <Button
                  type='button'
                  variant='destructive'
                  size='xs'
                  disabled={!props.canRebind}
                  title={
                    props.canRebind
                      ? t('Rebind Link publication')
                      : t('Sensitive channel write permission is required.')
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
              </div>
            ))}
          </div>
        </AlertDescription>
      </Alert>
      <LinkPublicationRebindDialog
        open={rebindTarget !== null}
        onOpenChange={(open) => {
          if (!open) setRebindTarget(null)
        }}
        publication={rebindTarget?.publication || null}
        proposedSKU={rebindTarget?.linkSKU || ''}
      />
    </>
  )
}
