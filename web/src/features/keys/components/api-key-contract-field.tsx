import { useQuery } from '@tanstack/react-query'
import { type ReactNode, useEffect, useMemo, useRef } from 'react'
import type { UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

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
import { ContractModelAvailability } from './contract-model-availability'

export function ApiKeyContractField(props: {
  form: UseFormReturn<ApiKeyFormValues>
  open: boolean
  /** True once the mutate drawer has applied its form defaults. */
  ready: boolean
  isUpdate: boolean
}) {
  const { t } = useTranslation()
  // Self-service contract list for binding an API key to one customer contract.
  const { data: contractsData, isError: contractsError } = useQuery({
    queryKey: ['self-customer-contract'],
    queryFn: getSelfCustomerContract,
    enabled: props.open,
    staleTime: 60 * 1000,
  })

  const contracts = useMemo(
    () => contractsData?.data?.contracts ?? [],
    [contractsData]
  )
  const selectedContractId = props.form.watch('contract_id')
  const modelLimits = props.form.watch('model_limits')
  // A manual selection (including explicitly unbinding) is never overwritten
  // by re-renders or data refreshes.
  const manualSelectionRef = useRef(false)
  const autoSelectRef = useRef(false)
  useEffect(() => {
    if (!props.open) {
      manualSelectionRef.current = false
      autoSelectRef.current = false
    }
  }, [props.open])

  // New-key default: when the loaded list holds exactly one contract and that
  // contract is enabled, select it once as the initial value. The decision is
  // based on the total contract count (not the filtered enabled count); a
  // failed or incomplete load never infers a default. Existing keys keep
  // their stored binding.
  useEffect(() => {
    if (!props.open || props.isUpdate || !props.ready) return
    if (manualSelectionRef.current || autoSelectRef.current) return
    if (!contractsData?.success || !contractsData.data) return
    if (contracts.length !== 1 || !contracts[0].enabled) return
    if ((Number(props.form.getValues('contract_id')) || 0) !== 0) return
    autoSelectRef.current = true
    props.form.setValue('contract_id', contracts[0].id, { shouldDirty: true })
  }, [
    props.open,
    props.isUpdate,
    props.ready,
    contractsData,
    contracts,
    props.form,
  ])

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
  // A failed load must never look like "no contract": the discount list is
  // simply unavailable until the contract can be read again.
  if (
    contractsError ||
    (contractsData && (!contractsData.success || !contractsData.data))
  ) {
    return (
      <FormItem>
        <FormLabel>{t('Customer contract')}</FormLabel>
        <FormDescription className='text-destructive'>
          {t('Contract discount details failed to load')}
        </FormDescription>
        <FormMessage />
      </FormItem>
    )
  }
  if (contracts.length === 0 && !Number(selectedContractId)) return null
  let preview: ReactNode = null
  if (selected && !selected.enabled) {
    preview = (
      <p className='text-muted-foreground text-sm'>
        {t(
          'This contract is disabled. Bound keys use their own group routing and pricing.'
        )}
      </p>
    )
  } else if (selected && selected.models.length === 0) {
    preview = (
      <p className='text-muted-foreground text-sm'>
        {t('This contract has no model rules')}
      </p>
    )
  } else if (selected) {
    preview = (
      <div className='max-h-64 overflow-y-auto rounded-md border'>
        {modelLimits.length > 0 && (
          <p className='text-muted-foreground p-3 text-sm'>
            {t('This key also restricts models to: {{models}}', {
              models: modelLimits?.join(', ') || t('None'),
            })}
          </p>
        )}
        <table className='w-full text-sm'>
          <thead>
            <tr className='text-muted-foreground border-b'>
              <th className='px-3 py-2 text-left font-medium'>{t('Model')}</th>
              <th className='px-3 py-2 text-left font-medium'>
                {t('Contract discount')}
              </th>
            </tr>
          </thead>
          <tbody className='divide-y'>
            {selected.models.map((rule) => (
              <tr key={rule.model}>
                <td className='px-3 py-2 font-mono break-all'>
                  {rule.model}
                  <ContractModelAvailability availability={rule.availability} />
                </td>
                <td className='px-3 py-2'>{rule.discount}</td>
              </tr>
            ))}
          </tbody>
        </table>
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
              onValueChange={(value) => {
                manualSelectionRef.current = true
                field.onChange(Number(value || 0))
              }}
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
              'An active contract defines model and channel scope and discounts. Disabling or unbinding it restores the key group routing and pricing.'
            )}
          </FormDescription>
          {preview}
          <FormMessage />
        </FormItem>
      )}
    />
  )
}
