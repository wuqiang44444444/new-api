import { useTranslation } from 'react-i18next'

import type { SelfCustomerContractRule } from '../types'

export function ContractModelAvailability({
  availability,
}: {
  availability: SelfCustomerContractRule['availability']
}) {
  const { t } = useTranslation()
  let label = ''
  if (availability === 'unavailable') label = t('No available contract channel')
  if (availability === 'group_denied') {
    label = t('Contract group access is unavailable')
  }
  if (availability === 'disabled') label = t('Contract mode is inactive')
  if (!label) return null
  return (
    <span className='text-muted-foreground block font-sans text-xs'>
      {label}
    </span>
  )
}
