import { useQuery } from '@tanstack/react-query'
import { useMemo, type ReactNode } from 'react'
import type { UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { ContractPriceDetails } from '@/components/contract-price-details'
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

import { getSelfCustomerContract } from '../api'
import type { ApiKeyFormValues } from '../lib'

export function ApiKeyContractField(props: {
  form: UseFormReturn<ApiKeyFormValues>
  open: boolean
}) {
  const { t } = useTranslation()
  // Self-service contract list for binding an API key to one customer contract.
  const { data: contractsData } = useQuery({
    queryKey: ['self-customer-contract'],
    queryFn: getSelfCustomerContract,
    enabled: props.open,
    staleTime: 60 * 1000,
  })

  // Self-service contract list for binding an API key to one customer contract.
  const contracts = useMemo(
    () => contractsData?.data?.contracts ?? [],
    [contractsData]
  )
  const selectedContractId = props.form.watch('contract_id')
  const contractOptions = useMemo(() => {
    const options = contracts
      .filter((contract) => contract.enabled)
      .map((contract) => ({
        value: String(contract.id),
        label: `${contract.name} · ${t('{{count}} models', {
          count: contract.models.length,
        })}`,
      }))
    const currentId = Number(selectedContractId) || 0
    // A key may stay bound to a contract that has since been disabled; keep
    // that binding visible so an edit does not silently unbind it.
    if (
      currentId > 0 &&
      !options.some((option) => option.value === String(currentId))
    ) {
      const bound = contracts.find((contract) => contract.id === currentId)
      options.unshift({
        value: String(currentId),
        label: bound
          ? `${bound.name} · ${t('{{count}} models', {
              count: bound.models.length,
            })} · ${t('Disabled')}`
          : `${t('Contract')} #${currentId} · ${t('Disabled')}`,
      })
    }
    return options
  }, [contracts, selectedContractId, t])

  const selected = contracts.find(
    (contract) => contract.id === Number(selectedContractId)
  )
  if (contracts.length === 0 && !Number(selectedContractId)) return null
  let preview: ReactNode = null
  if (selected && !selected.enabled) {
    preview = (
      <p className='text-muted-foreground text-sm'>
        {t(
          'This contract is disabled. Bound API keys currently follow native logic.'
        )}
      </p>
    )
  } else if (selected && selected.models.length === 0) {
    preview = (
      <p className='text-muted-foreground text-sm'>
        {t('No models are currently authorized')}
      </p>
    )
  } else if (selected) {
    preview = (
      <div className='max-h-64 divide-y overflow-y-auto rounded-md border'>
        {selected.models.map((rule) => (
          <div key={rule.model} className='min-w-0 space-y-1 p-3 text-sm'>
            <div className='font-mono break-all'>{rule.model}</div>
            <div className='text-muted-foreground'>
              {rule.available ? t('Available') : t('Unavailable')}
            </div>
            <ContractPriceDetails
              price={rule.price}
              channelMultiplier={rule.channel_discount || '1'}
              contractDiscount={rule.discount}
              effectiveMultiplier={rule.effective_multiplier || rule.discount}
            />
          </div>
        ))}
      </div>
    )
  }
  return (
    <FormField
      control={props.form.control}
      name='contract_id'
      render={({ field }) => (
        <FormItem>
          <FormLabel>{t('Customer contract')}</FormLabel>
          <FormControl>
            <Select
              items={[
                ...contractOptions,
                { value: '0', label: t('Do not bind a contract') },
              ]}
              value={String(field.value ?? 0)}
              onValueChange={(value) => field.onChange(Number(value || 0))}
            >
              <SelectTrigger className='w-full'>
                <SelectValue>
                  {Number(field.value ?? 0) === 0
                    ? t('Do not bind a contract')
                    : (contractOptions.find(
                        (option) => option.value === String(field.value ?? 0)
                      )?.label ?? t('Do not bind a contract'))}
                </SelectValue>
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  <SelectItem value='0'>
                    {t('Do not bind a contract')}
                  </SelectItem>
                  {contractOptions.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </FormControl>
          <FormDescription>
            {t(
              "Keys bound to a contract can only call that contract's models."
            )}
          </FormDescription>
          {preview}
          <FormMessage />
        </FormItem>
      )}
    />
  )
}
