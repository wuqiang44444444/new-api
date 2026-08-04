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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'

import { rebindLinkModelPublication } from '../../api'
import type { LinkModelPublication } from '../../types'

type LinkPublicationRebindDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  publication: LinkModelPublication | null
  proposedSKU: string
}

export function LinkPublicationRebindDialog(
  props: LinkPublicationRebindDialogProps
) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [reason, setReason] = useState('')
  const mutation = useMutation({
    mutationFn: rebindLinkModelPublication,
  })

  useEffect(() => {
    if (!props.open) setReason('')
  }, [props.open])

  const handleSubmit = async () => {
    const publication = props.publication
    if (!publication || !reason.trim()) return

    try {
      const response = await mutation.mutateAsync({
        contract_namespace: publication.contract_namespace,
        route_family: publication.route_family,
        customer_model: publication.customer_model,
        link_sku: props.proposedSKU,
        expected_version: publication.publication_version,
        reason: reason.trim(),
      })
      if (!response.success) {
        toast.error(response.message || t('Failed to rebind publication'))
        return
      }
      await queryClient.invalidateQueries({
        queryKey: ['link_model_publications'],
      })
      toast.success(t('Publication rebound successfully'))
      props.onOpenChange(false)
    } catch (error: unknown) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to rebind publication')
      )
    }
  }

  if (!props.publication) return null

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Rebind Link publication')}
      description={t(
        'This explicitly changes an immutable customer-model publication. The expected version and reason are recorded for audit.'
      )}
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button
            variant='outline'
            onClick={() => props.onOpenChange(false)}
            disabled={mutation.isPending}
          >
            {t('Cancel')}
          </Button>
          <Button
            variant='destructive'
            onClick={handleSubmit}
            disabled={!reason.trim() || mutation.isPending}
          >
            {mutation.isPending && (
              <Loader2 className='mr-2 h-4 w-4 animate-spin' />
            )}
            {t('Confirm rebind')}
          </Button>
        </>
      }
    >
      <dl className='grid grid-cols-[auto_1fr] gap-x-3 gap-y-2 text-sm'>
        <dt className='text-muted-foreground'>{t('Customer model')}</dt>
        <dd className='font-mono'>{props.publication.customer_model}</dd>
        <dt className='text-muted-foreground'>{t('Current Link SKU')}</dt>
        <dd className='font-mono'>{props.publication.link_sku}</dd>
        <dt className='text-muted-foreground'>{t('Proposed Link SKU')}</dt>
        <dd className='font-mono'>{props.proposedSKU}</dd>
        <dt className='text-muted-foreground'>{t('Expected version')}</dt>
        <dd>{props.publication.publication_version}</dd>
      </dl>
      <div className='space-y-2'>
        <Label htmlFor='link-publication-rebind-reason'>
          {t('Rebind reason')}
        </Label>
        <Textarea
          id='link-publication-rebind-reason'
          value={reason}
          onChange={(event) => setReason(event.target.value)}
          placeholder={t('Explain why this immutable publication must change')}
          disabled={mutation.isPending}
        />
      </div>
    </Dialog>
  )
}
