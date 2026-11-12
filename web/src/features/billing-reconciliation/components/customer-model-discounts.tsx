import { useTranslation } from 'react-i18next'

import { TableCell } from '@/components/ui/table'

import { formatDiscountFactor, formatDiscountTier } from '../lib'
import type { CustomerModelSummary } from '../types'

function ModelDiscountCell(props: { ratio?: number; multiple?: boolean }) {
  const { t } = useTranslation()
  const factor = props.ratio == null ? '' : formatDiscountFactor(props.ratio)

  let label = t('Not recorded')
  if (props.multiple) {
    label = t('Multiple discounts')
  } else if (factor) {
    label = factor
    if (props.ratio != null && props.ratio < 1) {
      label = `${formatDiscountTier(props.ratio, t)} ${factor}`
    }
  }

  return <TableCell className='text-right tabular-nums'>{label}</TableCell>
}

export function CustomerModelDiscounts(props: { model: CustomerModelSummary }) {
  return (
    <>
      <ModelDiscountCell
        ratio={props.model.contract_discount_ratio}
        multiple={props.model.multiple_contract_discounts}
      />
      <ModelDiscountCell
        ratio={props.model.discount_ratio}
        multiple={props.model.multiple_discounts}
      />
    </>
  )
}
