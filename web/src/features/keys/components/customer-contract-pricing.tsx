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
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from '@/components/ui/empty'
import { ContractPriceDetails } from '@/components/contract-price-details'

import { getSelfCustomerContract } from '../api'

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
  if (isError || !data?.success) {
    return (
      <Alert variant='destructive'>
        <AlertTitle>
          {t('Contract pricing is temporarily unavailable')}
        </AlertTitle>
        <AlertDescription>
          {t(
            'Model access remains fail-closed until the contract can be loaded.'
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
          <CardTitle>{t('Your model contract pricing')}</CardTitle>
          <CardDescription>
            {contracts.length === 1
              ? t('These rules apply only to API keys bound to this contract.')
              : t(
                  'Each API key follows the pricing of the contract bound to it.'
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
                          'This contract is disabled. Bound API keys currently follow native logic.'
                        )}
                      </AlertDescription>
                    </Alert>
                  )
                } else if (contract.models.length === 0) {
                  contractContent = (
                    <Empty className='border'>
                      <EmptyHeader>
                        <EmptyTitle>
                          {t('No models are currently authorized')}
                        </EmptyTitle>
                        <EmptyDescription>
                          {t(
                            'API keys bound to this contract cannot call models until rules are available.'
                          )}
                        </EmptyDescription>
                      </EmptyHeader>
                    </Empty>
                  )
                } else {
                  contractContent = (
                    <div className='divide-y rounded-lg border'>
                      {contract.models.map((rule) => {
                        const channelMultiplier = rule.channel_discount || '1'
                        const effectiveMultiplier =
                          rule.effective_multiplier || rule.discount
                        return (
                          <div
                            key={rule.model}
                            className='grid gap-2 p-3 md:grid-cols-[minmax(200px,1fr)_130px_minmax(240px,1fr)] md:items-start'
                          >
                            <div className='flex min-w-0 items-center gap-2'>
                              <span className='truncate font-mono text-sm'>
                                {rule.model}
                              </span>
                              <Badge
                                variant={
                                  rule.available ? 'secondary' : 'destructive'
                                }
                              >
                                {rule.available
                                  ? t('Available')
                                  : t('Unavailable')}
                              </Badge>
                            </div>
                            <div className='text-sm'>
                              <div>
                                {t('Contract discount')}: {rule.discount}
                              </div>
                              <div className='text-muted-foreground text-xs'>
                                {t('Channel multiplier')}: {channelMultiplier}
                              </div>
                            </div>
                            <div className='min-w-0 text-sm'>
                              <ContractPriceDetails
                                price={rule.price}
                                channelMultiplier={channelMultiplier}
                                contractDiscount={rule.discount}
                                effectiveMultiplier={effectiveMultiplier}
                              />
                            </div>
                          </div>
                        )
                      })}
                    </div>
                  )
                }
                return (
                  <div key={contract.id} className='flex flex-col gap-2'>
                    <div className='flex items-center gap-2'>
                      <span className='font-medium text-sm'>
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
