import { useQuery } from '@tanstack/react-query'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Collapsible, CollapsibleContent } from '@/components/ui/collapsible'
import { Empty, EmptyHeader, EmptyTitle } from '@/components/ui/empty'

import { getSelfCustomerContract } from '../api'
import { ContractModelAvailability } from './contract-model-availability'

export function CustomerContractPricing() {
  const { t } = useTranslation()
  const [open, setOpen] = useState(true)
  const { data, isLoading, isError } = useQuery({
    queryKey: ['self-customer-contract'],
    queryFn: getSelfCustomerContract,
    staleTime: 60 * 1000,
  })
  const contracts = data?.data?.contracts ?? []

  if (isLoading) {
    return (
      <div className='text-muted-foreground py-3 text-sm'>
        {t('Loading contract pricing...')}
      </div>
    )
  }
  if (isError || !data?.success || !data.data) {
    return (
      <Alert variant='destructive'>
        <AlertTitle>
          {t('Contract pricing is temporarily unavailable')}
        </AlertTitle>
        <AlertDescription>
          {t(
            'Discount details stay unavailable until the contract can be loaded.'
          )}
        </AlertDescription>
      </Alert>
    )
  }
  if (contracts.length === 0) return null

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <Card size='sm'>
        <CardHeader>
          <CardTitle>{t('Your model contract discounts')}</CardTitle>
          <CardDescription>
            {contracts.length === 1
              ? t('These discounts apply to API keys bound to this contract.')
              : t(
                  'Each API key follows the discounts of the contract bound to it.'
                )}
          </CardDescription>
          <CardAction>
            <Button
              variant='ghost'
              size='icon-sm'
              aria-label={
                open
                  ? t('Collapse contract pricing')
                  : t('Expand contract pricing')
              }
              onClick={() => setOpen((current) => !current)}
            >
              {open ? <ChevronDown /> : <ChevronRight />}
            </Button>
          </CardAction>
        </CardHeader>
        <CollapsibleContent>
          <CardContent>
            <div className='flex flex-col gap-4'>
              {contracts.map((contract) => {
                let contractContent
                if (!contract.enabled) {
                  contractContent = (
                    <Alert>
                      <AlertTitle>{t('Contract mode is inactive')}</AlertTitle>
                      <AlertDescription>
                        {t(
                          'This contract is disabled. Bound keys use their own group routing and pricing.'
                        )}
                      </AlertDescription>
                    </Alert>
                  )
                } else if (contract.models.length === 0) {
                  contractContent = (
                    <Empty className='border'>
                      <EmptyHeader>
                        <EmptyTitle>
                          {t('This contract has no model rules')}
                        </EmptyTitle>
                      </EmptyHeader>
                    </Empty>
                  )
                } else {
                  contractContent = (
                    <div className='overflow-hidden rounded-lg border'>
                      <table className='w-full text-sm'>
                        <thead>
                          <tr className='text-muted-foreground border-b'>
                            <th className='px-3 py-2 text-left font-medium'>
                              {t('Model')}
                            </th>
                            <th className='px-3 py-2 text-left font-medium'>
                              {t('Contract discount')}
                            </th>
                          </tr>
                        </thead>
                        <tbody className='divide-y'>
                          {contract.models.map((rule) => (
                            <tr key={rule.model}>
                              <td className='px-3 py-2 font-mono break-all'>
                                {rule.model}
                                <ContractModelAvailability
                                  availability={rule.availability}
                                />
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
                  <div key={contract.id} className='flex flex-col gap-2'>
                    <div className='flex items-center gap-2'>
                      <span className='text-sm font-medium'>
                        {contract.name}
                      </span>
                      <Badge
                        variant={contract.enabled ? 'secondary' : 'outline'}
                      >
                        {contract.enabled ? t('Enabled') : t('Disabled')}
                      </Badge>
                    </div>
                    {contractContent}
                  </div>
                )
              })}
            </div>
          </CardContent>
        </CollapsibleContent>
      </Card>
    </Collapsible>
  )
}
