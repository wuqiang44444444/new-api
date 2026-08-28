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
import { KeyRound, ShieldAlert } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'

import {
  ASSET_TENANT_BOUNDARY_FIELD_LABELS,
  type AssetTenantBoundaryChange,
} from '../../lib'

type AssetTenantConfirmationMode = 'rotation' | 'replacement'

type AssetTenantRotationDialogProps = {
  open: boolean
  mode: AssetTenantConfirmationMode
  models: string[]
  boundaryChanges: AssetTenantBoundaryChange[]
  onCancel: () => void
  onConfirm: () => void
}

export function AssetTenantRotationDialog(
  props: AssetTenantRotationDialogProps
) {
  const { t } = useTranslation()
  const [replacementAccepted, setReplacementAccepted] = useState(false)
  const isReplacement = props.mode === 'replacement'

  const handleCancel = () => {
    setReplacementAccepted(false)
    props.onCancel()
  }

  const handleConfirm = () => {
    if (isReplacement && !replacementAccepted) return
    setReplacementAccepted(false)
    props.onConfirm()
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => {
        if (!open) handleCancel()
      }}
      title={
        <>
          {isReplacement ? (
            <ShieldAlert className='size-5' aria-hidden='true' />
          ) : (
            <KeyRound className='size-5' aria-hidden='true' />
          )}
          {t(
            isReplacement
              ? 'Confirm asset tenant replacement'
              : 'Confirm asset credential rotation'
          )}
        </>
      }
      titleClassName={
        isReplacement
          ? 'text-destructive flex items-center gap-2'
          : 'flex items-center gap-2'
      }
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button type='button' variant='outline' onClick={handleCancel}>
            {t('Cancel')}
          </Button>
          <Button
            type='button'
            variant={isReplacement ? 'destructive' : 'default'}
            disabled={isReplacement && !replacementAccepted}
            onClick={handleConfirm}
          >
            {t(
              isReplacement
                ? 'Confirm replacement and save'
                : 'Confirm tenant is unchanged'
            )}
          </Button>
        </>
      }
    >
      <Alert>
        <AlertTitle>{t('Asset library boundary: this channel')}</AlertTitle>
        <AlertDescription>
          {t(
            isReplacement
              ? 'The customer models and channel ID will remain unchanged. New asset operations will use the replacement tenant, while existing asset IDs and asset references may become unavailable. The platform will not migrate or delete old assets.'
              : 'Only continue when these credentials belong to the same upstream asset tenant. If the account or tenant changed, cancel and replace the asset tenant instead.'
          )}
        </AlertDescription>
      </Alert>
      {isReplacement && props.boundaryChanges.length > 0 ? (
        <div className='border-border rounded-lg border p-3'>
          <p className='mb-2 text-sm font-medium'>
            {t('Asset tenant boundary changes')}
          </p>
          <dl className='space-y-2 text-sm'>
            {props.boundaryChanges.map((change) => (
              <div
                key={change.field}
                className='grid gap-1 sm:grid-cols-[12rem_1fr]'
              >
                <dt className='text-muted-foreground'>
                  {t(ASSET_TENANT_BOUNDARY_FIELD_LABELS[change.field])}
                </dt>
                <dd className='font-mono text-xs break-all'>
                  {change.previous || t('Not set')} →{' '}
                  {change.next || t('Not set')}
                </dd>
              </div>
            ))}
          </dl>
        </div>
      ) : null}
      <div className='space-y-1 text-sm'>
        <p className='font-medium'>{t('Models sharing this asset library')}</p>
        <p className='text-muted-foreground break-words'>
          {props.models.length > 0
            ? props.models.join(', ')
            : t('None selected')}
        </p>
      </div>
      {isReplacement ? (
        <div className='flex items-start gap-2'>
          <Checkbox
            id='asset-tenant-replacement-risk'
            checked={replacementAccepted}
            onCheckedChange={(checked) =>
              setReplacementAccepted(checked === true)
            }
          />
          <Label
            htmlFor='asset-tenant-replacement-risk'
            className='text-sm leading-tight'
          >
            {t(
              'I understand that existing asset IDs and asset references may no longer be available.'
            )}
          </Label>
        </div>
      ) : null}
    </Dialog>
  )
}
