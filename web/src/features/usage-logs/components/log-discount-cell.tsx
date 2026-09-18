import type { TFunction } from 'i18next'

import { buildDiscountDisplay } from '../lib/discount-display'
import type { LogOtherData } from '../types'

type Translate = (key: string, opts?: Record<string, unknown>) => string

function FactorLine(props: { label: string; tier: string; factor: string }) {
  return (
    <>
      <dt className='text-muted-foreground whitespace-nowrap'>{props.label}</dt>
      <dd className='min-w-0 wrap-anywhere whitespace-normal tabular-nums'>
        {props.tier || '—'}
        {props.factor && props.factor !== props.tier && (
          <span className='text-muted-foreground'> {props.factor}</span>
        )}
      </dd>
    </>
  )
}

// 折扣及合同状态完整呈现；最小内容宽度避免标签挤占数值空间。
export function LogDiscountCell(props: {
  other: LogOtherData | null
  t: TFunction
}) {
  const { other, t } = props
  const translate: Translate = (key, opts) => t(key, opts)
  const discount = buildDiscountDisplay(other, translate)
  if (!discount.visible) {
    return <span className='text-muted-foreground text-xs'>—</span>
  }
  return (
    <dl className='grid min-w-60 grid-cols-[max-content_minmax(0,1fr)] items-start gap-x-3 gap-y-1 text-xs leading-relaxed'>
      <FactorLine
        label={t('Group discount/ratio')}
        tier={discount.groupTier}
        factor={discount.groupFactor}
      />
      <dt className='text-muted-foreground whitespace-nowrap'>
        {t('Contract discount')}
      </dt>
      <dd className='min-w-0 wrap-anywhere whitespace-normal'>
        {discount.contractState === 'applied' && discount.contractTier ? (
          <>
            <span className='block tabular-nums'>
              {discount.contractTier}
              {discount.contractFactor !== discount.contractTier && (
                <span className='text-muted-foreground'>
                  {' '}
                  {discount.contractFactor}
                </span>
              )}
            </span>
            <span className='text-muted-foreground block wrap-anywhere whitespace-normal'>
              {discount.contractLabel}
            </span>
          </>
        ) : (
          discount.contractLabel
        )}
      </dd>
      <FactorLine
        label={t('Final discount')}
        tier={discount.finalTier}
        factor={discount.finalFactor}
      />
    </dl>
  )
}
