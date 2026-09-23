import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverDescription,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  combinedDiscountFactor,
  formatCustomerStatementQuota,
  formatDiscountFactor,
  formatDiscountTier,
} from '@/features/billing-reconciliation/lib'
import type { BillingDiscountCombination } from '@/features/billing-reconciliation/types'

export function UsageDiscountCells(props: {
  combinations?: BillingDiscountCombination[]
  moneyIncomplete?: boolean
}) {
  const { t } = useTranslation()
  const combinations = props.combinations ?? []
  if (props.moneyIncomplete || combinations.length === 0) {
    const reason = props.moneyIncomplete
      ? t('Discount details await complete settlement records.')
      : t('Not recorded')
    return (
      <>
        <td className='text-muted-foreground py-2 pr-3'>{t('Not recorded')}</td>
        <td className='text-muted-foreground py-2 pr-3'>{t('Not recorded')}</td>
        <td className='text-muted-foreground py-2 pr-3 text-xs'>{reason}</td>
        <td className='py-2 pr-3'>—</td>
      </>
    )
  }
  if (combinations.length === 1) {
    return <UsageDiscountCombination combination={combinations[0]} inline />
  }
  const savings = combinations.reduce<number | undefined>((total, combo) => {
    if (total == null || combo.discount_quota == null) return undefined
    return total + combo.discount_quota
  }, 0)
  return (
    <>
      <td className='text-muted-foreground py-2 pr-3'>
        {t('Multiple discounts')}
      </td>
      <td className='text-muted-foreground py-2 pr-3'>
        {t('Multiple discounts')}
      </td>
      <td className='py-2 pr-3'>
        <Popover>
          <PopoverTrigger render={<Button variant='outline' size='xs' />}>
            {t('Discount details')} ({combinations.length})
          </PopoverTrigger>
          <PopoverContent
            align='end'
            className='max-h-[min(36rem,80dvh)] w-[64rem] max-w-[calc(100vw-2rem)] overflow-y-auto whitespace-normal'
          >
            <PopoverTitle>
              {t('Discount details')} · {t('Multiple discounts')}
            </PopoverTitle>
            <PopoverDescription className='text-xs'>
              {t(
                'Discounts reflect historical billing records. Model fees use the group ratio and contract discount; additional fees are separate.'
              )}
            </PopoverDescription>
            {combinations.map((combination) => (
              <UsageDiscountCombination
                key={JSON.stringify([
                  combination.other,
                  combination.group_id,
                  combination.model_name,
                  combination.billing_mode,
                  combination.group_name,
                  combination.group_ratio_source,
                  combination.group_ratio,
                  combination.contract_applicable,
                  combination.contract_id_known,
                  combination.contract_id,
                  combination.contract_version,
                  combination.contract_ratio,
                ])}
                combination={combination}
              />
            ))}
          </PopoverContent>
        </Popover>
      </td>
      <td className='py-2 pr-3 whitespace-nowrap'>
        {formatCustomerStatementQuota(savings)}
      </td>
    </>
  )
}

function UsageDiscountCombination(props: {
  combination: BillingDiscountCombination
  inline?: boolean
}) {
  const { t } = useTranslation()
  const combo = props.combination
  const invalid =
    combo.other || combo.estimate_reasons?.includes('invalid_facts')
  // Missing contract history may be used as ×1 for legacy price estimates,
  // but is never presented as evidence that no contract discount applied.
  const contractRatio =
    combo.contract_applicable === 'no' ? 1 : combo.contract_ratio
  const final =
    invalid || !['yes', 'no'].includes(combo.contract_applicable)
      ? null
      : combinedDiscountFactor(combo.group_ratio, contractRatio)
  const groupValue =
    !invalid && combo.group_ratio != null
      ? `${formatDiscountTier(combo.group_ratio, t)} (${formatDiscountFactor(combo.group_ratio)})`
      : t('Not recorded')
  let contractValue = t('Not recorded')
  if (!invalid && combo.contract_applicable === 'no') {
    contractValue = t('No contract discount applied')
  }
  if (
    !invalid &&
    combo.contract_applicable === 'yes' &&
    combo.contract_ratio != null
  ) {
    contractValue = `${formatDiscountTier(combo.contract_ratio, t)} (${formatDiscountFactor(combo.contract_ratio)})`
  }
  const group = (
    <div className='flex flex-col text-xs'>
      {combo.other && <span>{t('Other combinations')}</span>}
      <span className='font-medium'>{groupValue}</span>
      {combo.group_name && (
        <span className='mt-1 break-all'>{combo.group_name}</span>
      )}
      {combo.group_ratio_source === 'user_exclusive' && (
        <span>{t('User Exclusive Ratio')}</span>
      )}
    </div>
  )
  const contract = (
    <div className='flex flex-col text-xs'>
      <span className='font-medium'>{contractValue}</span>
      {combo.contract_applicable === 'yes' && (
        <span className='mt-1 break-all'>
          {combo.contract_name ||
            (combo.contract_id_known
              ? t('Contract #{{id}}', { id: combo.contract_id })
              : t('Historical identity not recorded'))}
          {combo.contract_version ? ` · v${combo.contract_version}` : ''}
        </span>
      )}
    </div>
  )
  const finalValue =
    final == null
      ? t('Not recorded')
      : `${formatDiscountTier(final, t)} (${formatDiscountFactor(final)})`
  const auxiliaryNote = combo.estimate_reasons?.includes(
    'auxiliary_charge'
  ) && (
    <p className='text-muted-foreground mt-1 text-xs whitespace-normal'>
      {t('Additional fees are not included in the model discount.')}
    </p>
  )
  if (props.inline) {
    return (
      <>
        <td className='py-2 pr-3'>{group}</td>
        <td className='py-2 pr-3'>{contract}</td>
        <td className='py-2 pr-3 text-xs whitespace-nowrap'>{finalValue}</td>
        <td className='py-2 pr-3 whitespace-nowrap'>
          {formatCustomerStatementQuota(combo.discount_quota)}
          {auxiliaryNote}
        </td>
      </>
    )
  }
  return (
    <div className='bg-muted/30 rounded-md border p-3 text-xs'>
      {combo.other && (
        <p className='mb-2 font-medium'>{t('Other combinations')}</p>
      )}
      <dl className='grid gap-3 sm:grid-cols-3 lg:grid-cols-6'>
        <div>
          <dt className='text-muted-foreground'>{t('Group discount/ratio')}</dt>
          <dd>{group}</dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>{t('Contract discount')}</dt>
          <dd>{contract}</dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>{t('Final discount')}</dt>
          <dd className='mt-1 font-medium'>{finalValue}</dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>{t('Estimated list price')}</dt>
          <dd className='mt-1 font-medium'>
            {formatCustomerStatementQuota(combo.original_quota)}
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>{t('Estimated savings')}</dt>
          <dd className='mt-1 font-medium'>
            {formatCustomerStatementQuota(combo.discount_quota)}
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>{t('Net amount')}</dt>
          <dd className='mt-1 font-medium'>
            {formatCustomerStatementQuota(combo.usage.net_quota)}
          </dd>
          <dd className='text-muted-foreground mt-1'>
            {t('Refund')}:{' '}
            {formatCustomerStatementQuota(combo.usage.refund_quota)}
          </dd>
        </div>
      </dl>
      {auxiliaryNote}
    </div>
  )
}
