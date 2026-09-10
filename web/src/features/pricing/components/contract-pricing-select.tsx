import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { getSelfCustomerContract } from '@/features/keys/api'
import { useAuthStore } from '@/stores/auth-store'

export function ContractPricingSelect({
  value,
  onChange,
}: {
  value: number | 'batch' | null
  onChange: (id: number | 'batch' | null) => void
}) {
  const { t } = useTranslation()
  const userId = useAuthStore((state) => state.auth.user?.id)
  const query = useQuery({
    queryKey: ['self-customer-contract', userId],
    queryFn: getSelfCustomerContract,
    enabled: Boolean(userId),
  })
  const contracts = query.data?.data?.contracts ?? []
  return (
    <div className='my-3 flex items-center justify-center gap-2'>
      <label htmlFor='pricing-contract'>{t('Pricing scope')}</label>
      <select
        id='pricing-contract'
        className='border-input bg-background rounded-md border p-2'
        value={value ?? ''}
        onChange={(event) => {
          const selected = event.target.value
          if (selected === 'batch') onChange('batch')
          else onChange(selected ? Number(selected) : null)
        }}
      >
        <option value=''>{t('Native pricing')}</option>
        <option value='batch'>{t('Azure Batch')}</option>
        {contracts
          .filter((contract) => contract.enabled)
          .map((contract) => (
            <option key={contract.id} value={contract.id}>
              {contract.name}
            </option>
          ))}
      </select>
      {query.isError && (
        <span role='alert'>
          {t('Contract pricing is temporarily unavailable')}
        </span>
      )}
    </div>
  )
}
