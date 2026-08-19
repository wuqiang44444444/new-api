import { ChevronDown, ChevronRight } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Collapsible, CollapsibleContent } from '@/components/ui/collapsible'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'

export type ContractPricePreview = {
  price_type: 'model_price' | 'model_ratio' | 'tiered_multiplier'
  billing_mode?: string
  base_model_price?: string
  final_model_price?: string
  base_model_ratio?: string
  final_model_ratio?: string
  completion_ratio?: string
  base_image_ratio?: string
  final_image_ratio?: string
  current_discounted_price?: string
}

type ContractPriceDetailsProps = {
  price: ContractPricePreview
  channelMultiplier: string
  contractDiscount: string
  effectiveMultiplier: string
  compact?: boolean
}

// 1 ratio unit equals $2 per 1M tokens — the same convention as the
// usage-log billing breakdown (model_ratio * 2.0).
const USD_PER_MILLION_PER_RATIO = 2.0
const PRICE_FORMAT_OPTIONS = { digitsLarge: 4, digitsSmall: 6, abbreviate: false }

function toNumber(value: string | undefined): number {
  if (!value) return Number.NaN
  return Number(value)
}

function resolveFinalValue(
  finalValue: string | undefined,
  baseValue: number,
  effective: number
): number {
  const final = toNumber(finalValue)
  if (Number.isFinite(final)) return final
  if (Number.isFinite(baseValue) && Number.isFinite(effective)) {
    return baseValue * effective
  }
  return Number.NaN
}

function formatRatio(value: string | undefined): string {
  const number = toNumber(value)
  if (!Number.isFinite(number)) return value || '—'
  return `${number.toFixed(8).replace(/\.?0+$/, '')}x`
}

function formatScalar(value: string | undefined): string {
  const number = toNumber(value)
  if (!Number.isFinite(number)) return value || '—'
  return number.toFixed(8).replace(/\.?0+$/, '')
}

function formatUsdPerMillion(ratio: number): string {
  return `${formatBillingCurrencyFromUSD(
    ratio * USD_PER_MILLION_PER_RATIO,
    PRICE_FORMAT_OPTIONS
  )}/M`
}

function formatUsd(amount: number): string {
  return formatBillingCurrencyFromUSD(amount, PRICE_FORMAT_OPTIONS)
}

function formatTokenAmount(value: string | undefined): string {
  const number = toNumber(value)
  if (!Number.isFinite(number)) return value || '—'
  return (number * 100).toFixed(8).replace(/\.?0+$/, '')
}

type PriceRowProps = { label: string; value: string }

function PriceRow(props: PriceRowProps) {
  return (
    <div className='flex justify-between gap-3'>
      <span className='text-muted-foreground'>{props.label}</span>
      <span className='font-mono tabular-nums'>{props.value}</span>
    </div>
  )
}

export function ContractPriceDetails(props: ContractPriceDetailsProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const isTiered = props.price.price_type === 'tiered_multiplier'

  const completion = toNumber(props.price.completion_ratio)
  const hasCompletion = Number.isFinite(completion) && completion > 0
  const effective = toNumber(props.effectiveMultiplier)

  const baseInputRatio = toNumber(props.price.base_model_ratio)
  const finalInputRatio = resolveFinalValue(props.price.final_model_ratio, baseInputRatio, effective)
  const baseOutputRatio = hasCompletion ? baseInputRatio * completion : Number.NaN
  const finalOutputRatio = hasCompletion ? finalInputRatio * completion : Number.NaN
  const baseImageRatio = toNumber(props.price.base_image_ratio)
  const finalImageRatio = resolveFinalValue(props.price.final_image_ratio, baseImageRatio, effective)

  const baseCallPrice = toNumber(props.price.base_model_price)
  const finalCallPrice = resolveFinalValue(props.price.final_model_price, baseCallPrice, effective)

  const finalTokenParts: string[] = []
  if (finalInputRatio > 0) {
    finalTokenParts.push(`${t('Input')} ${formatUsdPerMillion(finalInputRatio)}`)
  }
  if (hasCompletion && finalOutputRatio > 0) {
    finalTokenParts.push(`${t('Output')} ${formatUsdPerMillion(finalOutputRatio)}`)
  }
  if (finalImageRatio > 0) {
    finalTokenParts.push(`${t('Image')} ${formatUsdPerMillion(finalImageRatio)}`)
  }

  let summary = '—'
  if (isTiered) {
    const tierValue =
      props.price.final_model_price ||
      props.price.final_model_ratio ||
      props.price.final_image_ratio ||
      props.price.current_discounted_price ||
      props.contractDiscount
    if (!tierValue) {
      summary = t('Tiered pricing')
    } else if (Number.isFinite(Number(tierValue))) {
      summary = t('Tiered price × {{discount}}', {
        discount: formatScalar(tierValue),
      })
    } else {
      summary = tierValue
    }
  } else if (props.price.price_type === 'model_price') {
    summary = Number.isFinite(finalCallPrice)
      ? `${formatUsd(finalCallPrice)} / ${t('request')}`
      : '—'
  } else if (finalTokenParts.length > 0) {
    summary = finalTokenParts.join(' · ')
  } else {
    const finalRatio = props.price.final_model_ratio || props.price.current_discounted_price
    if (finalRatio) {
      summary = `${formatScalar(props.channelMultiplier)} × ${formatScalar(props.contractDiscount)} = ${formatRatio(finalRatio)}`
    } else {
      summary = `${formatScalar(props.channelMultiplier)} × ${formatScalar(props.contractDiscount)} = ${formatRatio(props.effectiveMultiplier)}`
    }
  }

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <div className='flex min-w-0 items-center gap-1.5'>
        <span className={props.compact ? 'min-w-0 truncate text-xs' : 'min-w-0 truncate text-sm'}>
          {summary}
        </span>
        <Button
          type='button'
          variant='ghost'
          size='icon-sm'
          className='size-6 shrink-0'
          aria-label={open ? t('Hide pricing details') : t('Show pricing details')}
          onClick={() => setOpen((current) => !current)}
        >
          {open ? <ChevronDown /> : <ChevronRight />}
        </Button>
      </div>
      <CollapsibleContent>
        <div className='bg-muted/30 mt-2 space-y-1.5 rounded-md border p-2 text-xs'>
          {props.price.price_type === 'model_price' && (
            <>
              {Number.isFinite(baseCallPrice) && (
                <PriceRow label={t('Base request price')} value={formatUsd(baseCallPrice)} />
              )}
              <PriceRow label={t('Channel multiplier')} value={formatRatio(props.channelMultiplier)} />
              <PriceRow label={t('Contract discount')} value={formatRatio(props.contractDiscount)} />
              <PriceRow label={t('Effective multiplier')} value={formatRatio(props.effectiveMultiplier)} />
              {Number.isFinite(finalCallPrice) && (
                <div className='flex justify-between gap-3 border-t pt-1.5 font-medium'>
                  <span>{t('Final request price')}</span>
                  <span className='font-mono tabular-nums'>{formatUsd(finalCallPrice)}</span>
                </div>
              )}
            </>
          )}
          {props.price.price_type === 'model_ratio' && (
            <>
              {baseInputRatio > 0 && (
                <PriceRow label={t('Base input price')} value={formatUsdPerMillion(baseInputRatio)} />
              )}
              {hasCompletion && baseOutputRatio > 0 && (
                <PriceRow label={t('Base output price')} value={formatUsdPerMillion(baseOutputRatio)} />
              )}
              {baseImageRatio > 0 && (
                <PriceRow label={t('Base image token price')} value={formatUsdPerMillion(baseImageRatio)} />
              )}
              <PriceRow label={t('Channel multiplier')} value={formatRatio(props.channelMultiplier)} />
              <PriceRow label={t('Contract discount')} value={formatRatio(props.contractDiscount)} />
              <PriceRow label={t('Effective multiplier')} value={formatRatio(props.effectiveMultiplier)} />
              {(finalInputRatio > 0 || (hasCompletion && finalOutputRatio > 0) || finalImageRatio > 0) && (
                <div className='border-t pt-1.5 font-medium'>
                  {finalInputRatio > 0 && (
                    <PriceRow label={t('Final input price')} value={formatUsdPerMillion(finalInputRatio)} />
                  )}
                  {hasCompletion && finalOutputRatio > 0 && (
                    <PriceRow label={t('Final output price')} value={formatUsdPerMillion(finalOutputRatio)} />
                  )}
                  {finalImageRatio > 0 && (
                    <PriceRow label={t('Final image token price')} value={formatUsdPerMillion(finalImageRatio)} />
                  )}
                </div>
              )}
              {baseImageRatio > 0 && finalImageRatio > 0 && (
                <p className='text-muted-foreground border-t pt-1.5 leading-relaxed'>
                  {t(
                    'Every 100 image billing tokens are priced as {{base}} standard tokens; with a final multiplier of {{multiplier}}, the final price is {{final}} standard tokens.',
                    {
                      base: formatTokenAmount(props.price.base_image_ratio),
                      multiplier: formatScalar(props.effectiveMultiplier),
                      final: formatTokenAmount(props.price.final_image_ratio),
                    }
                  )}
                </p>
              )}
            </>
          )}
        </div>
      </CollapsibleContent>
    </Collapsible>
  )
}
