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
import { KeyRound } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'

type AssetTenantRotationDialogProps = {
  open: boolean
  models: string[]
  onCancel: () => void
  onConfirm: () => void
}

export function AssetTenantRotationDialog(
  props: AssetTenantRotationDialogProps
) {
  const { t } = useTranslation()

  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => {
        if (!open) props.onCancel()
      }}
      title={
        <>
          <KeyRound className='size-5' aria-hidden='true' />
          {t('Confirm asset credential rotation')}
        </>
      }
      titleClassName='flex items-center gap-2'
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button type='button' variant='outline' onClick={props.onCancel}>
            {t('Cancel')}
          </Button>
          <Button type='button' onClick={props.onConfirm}>
            {t('Confirm tenant is unchanged')}
          </Button>
        </>
      }
    >
      <Alert>
        <AlertTitle>{t('Asset library boundary: this channel')}</AlertTitle>
        <AlertDescription>
          {t(
            'Only continue when these credentials belong to the same upstream asset tenant. If the account or tenant changed, cancel and create a new channel.'
          )}
        </AlertDescription>
      </Alert>
      <div className='space-y-1 text-sm'>
        <p className='font-medium'>{t('Models sharing this asset library')}</p>
        <p className='text-muted-foreground break-words'>
          {props.models.length > 0
            ? props.models.join(', ')
            : t('None selected')}
        </p>
      </div>
    </Dialog>
  )
}
