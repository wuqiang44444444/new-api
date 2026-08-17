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

import { getSelfCustomerContract } from '../api'
import type { SelfCustomerContractRule } from '../types'

function ContractPrice({ rule }: { rule: SelfCustomerContractRule }) {
  const { t } = useTranslation()
  if (rule.price.price_type === 'tiered_multiplier') {
    return (
      <span>
        {t('Tiered price × {{discount}}', { discount: rule.discount })}
      </span>
    )
  }
  return (
    <span>
      {rule.price.current_discounted_price || '—'}{' '}
      <span className='text-muted-foreground'>
        (
        {rule.price.price_type === 'model_price'
          ? t('Fixed model price')
          : t('Token price ratio')}
        )
      </span>
    </span>
  )
}

export function CustomerContractPricing() {
  const { t } = useTranslation()
  const [open, setOpen] = useState(true)
  const { data, isLoading, isError } = useQuery({
    queryKey: ['self-customer-contract'],
    queryFn: getSelfCustomerContract,
    staleTime: 60 * 1000,
  })
  const contract = data?.data

  if (isLoading) {
    return (
      <div className='text-muted-foreground py-3 text-sm'>
        {t('Loading contract pricing...')}
      </div>
    )
  }
  if (isError || !data?.success || !contract) {
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
  if (!contract.contract_mode && contract.contract_version === 0) return null

  let contractContent
  if (!contract.contract_mode) {
    contractContent = (
      <Alert>
        <AlertTitle>{t('Contract mode is inactive')}</AlertTitle>
      </Alert>
    )
  } else if (contract.models.length === 0) {
    contractContent = (
      <Empty className='border'>
        <EmptyHeader>
          <EmptyTitle>{t('No models are currently authorized')}</EmptyTitle>
          <EmptyDescription>
            {t('All model calls from every API key are currently denied.')}
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  } else {
    contractContent = (
      <div className='divide-y rounded-lg border'>
        {contract.models.map((rule) => (
          <div
            key={rule.model}
            className='grid gap-2 p-3 md:grid-cols-[minmax(200px,1fr)_120px_minmax(180px,1fr)] md:items-center'
          >
            <div className='flex min-w-0 items-center gap-2'>
              <span className='truncate font-mono text-sm'>{rule.model}</span>
              <Badge variant={rule.available ? 'secondary' : 'destructive'}>
                {rule.available ? t('Available') : t('Unavailable')}
              </Badge>
            </div>
            <div className='text-sm'>
              {t('Discount')}: {rule.discount}
            </div>
            <div className='text-sm'>
              <ContractPrice rule={rule} />
            </div>
          </div>
        ))}
      </div>
    )
  }

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <Card size='sm'>
        <CardHeader>
          <CardTitle>{t('Your model contract pricing')}</CardTitle>
          <CardDescription>
            {contract.contract_mode
              ? t('These model and price rules apply to all of your API keys.')
              : t(
                  'Contract mode is inactive; native model permissions and pricing currently apply.'
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
          <CardContent>{contractContent}</CardContent>
        </CollapsibleContent>
      </Card>
    </Collapsible>
  )
}
