import { useMutation, useQuery } from '@tanstack/react-query'
import {
  useEffect,
  useImperativeHandle,
  useRef,
  useState,
  type ReactNode,
  type Ref,
} from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { previewBillingExpressions } from '@/features/pricing/lib/billing-display-preview'
import { useDebounce } from '@/hooks/use-debounce'
import { api } from '@/lib/api'

import {
  hasImagePricingIssue,
  imagePriceWarningText,
  type ImagePriceWarning,
} from './image-price-warnings'
import { PricingAmountInput } from './pricing-amount-input'

type PriceTable = {
  profile: string
  prices: Record<string, number> | null
  qualities: string[]
  size_tiers: string[]
  default_quality: string
  unmatched_uses_quality: boolean
}

type ImagePricingProjection = {
  importable?: boolean
  table?: PriceTable | null
  profiles?: PriceTable[]
  warnings?: ImagePriceWarning[]
  expression?: string
}

async function imagePrices(request: object): Promise<ImagePricingProjection> {
  const { data } = await api.post(
    '/api/option/billing-expression/image-prices',
    request
  )
  if (!data.success) throw new Error(data.message)
  return data.data
}

export type ImagePriceGridHandle = {
  commit: () => Promise<string | undefined | null>
}

export function ImageSizePricingEditor(props: {
  expression: string
  hasRequestRules?: boolean
  onExpressionChange: (expression: string) => void
  onDirtyChange: (dirty: boolean) => void
  children: ReactNode
  ref?: Ref<ImagePriceGridHandle>
}) {
  const { t } = useTranslation()
  const expression = useDebounce(props.expression, 250)
  const [raw, setRaw] = useState(false)
  const [draft, setDraft] = useState<PriceTable>()
  const [choosing, setChoosing] = useState(false)
  const grid = useRef<ImagePriceGridHandle>(null)
  useImperativeHandle(
    props.ref,
    () => ({ commit: async () => grid.current?.commit() }),
    []
  )
  const projection = useQuery({
    queryKey: ['image-size-pricing', expression],
    queryFn: () => imagePrices({ expression }),
    enabled:
      expression.length <= 20000 &&
      (choosing || expression.includes('image_count')),
  })
  const migration = useMutation({
    mutationFn: () =>
      imagePrices({ expression: props.expression, migrate: true }),
    onSuccess: (result) => {
      if (result.table) {
        setDraft(result.table)
        setRaw(false)
      }
    },
  })
  const saved =
    expression === props.expression ? projection.data?.table : undefined
  const table = draft ?? saved
  const warnings =
    expression === props.expression ? projection.data?.warnings : undefined

  return (
    <div className='min-w-0 space-y-3'>
      {!!warnings?.length && (
        <Alert
          variant={hasImagePricingIssue(warnings) ? 'destructive' : 'default'}
        >
          <AlertDescription>
            {warnings.map((warning) => (
              <p key={warning.code + (warning.field ?? '')}>
                {imagePriceWarningText(warning)}
              </p>
            ))}
          </AlertDescription>
        </Alert>
      )}
      {props.hasRequestRules && props.children}
      {!props.hasRequestRules &&
        (table && !raw ? (
          <>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={async () => {
                const edited = await grid.current?.commit()
                if (edited === null) return
                if (edited !== undefined) props.onExpressionChange(edited)
                setDraft(undefined)
                setRaw(true)
                props.onDirtyChange(false)
              }}
            >
              {t('Edit expression')}
            </Button>
            <ImagePriceGrid
              key={table.profile + props.expression}
              ref={grid}
              table={table}
              isDraft={!!draft}
              onDirtyChange={props.onDirtyChange}
            />
          </>
        ) : (
          <>
            <div className='flex flex-wrap gap-2'>
              {saved && (
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={() => setRaw(false)}
                >
                  {t('Size tiers per image')}
                </Button>
              )}
              {!saved && (
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={() => setChoosing(!choosing)}
                  aria-expanded={choosing}
                >
                  {t('Size tiers per image')}
                </Button>
              )}
              {expression === props.expression &&
                projection.data?.importable && (
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    disabled={migration.isPending}
                    onClick={() => migration.mutate()}
                  >
                    {t('Import existing image prices')}
                  </Button>
                )}
            </div>
            {(migration.isError ||
              (migration.isSuccess && !migration.data.table)) && (
              <p role='alert' className='text-destructive text-sm'>
                {t('Could not import image prices.')}
              </p>
            )}
            {choosing && projection.isPending && (
              <p role='status'>{t('Loading...')}</p>
            )}
            {choosing && projection.isError && (
              <p role='alert'>{t('Could not load image pricing.')}</p>
            )}
            {choosing && !saved && (
              <div className='flex flex-wrap gap-2'>
                {projection.data?.profiles?.map((profile) => (
                  <Button
                    type='button'
                    key={profile.profile}
                    variant='outline'
                    size='sm'
                    onClick={() => {
                      setChoosing(false)
                      setRaw(false)
                      setDraft(profile)
                    }}
                  >
                    {profile.qualities.length === 1
                      ? t('Size only')
                      : t('{{count}} qualities · Default {{quality}}', {
                          count: profile.qualities.length,
                          quality: profile.default_quality,
                        })}
                  </Button>
                ))}
              </div>
            )}
            {props.children}
          </>
        ))}
    </div>
  )
}

function ImagePriceGrid(props: {
  table: PriceTable
  isDraft: boolean
  onDirtyChange: (dirty: boolean) => void
  ref: Ref<ImagePriceGridHandle>
}) {
  const { t } = useTranslation()
  const [prices, setPrices] = useState<Record<string, string>>(() =>
    Object.fromEntries(
      Object.entries(props.table.prices ?? {}).map(([key, price]) => [
        key,
        String(price),
      ])
    )
  )
  const [size, setSize] = useState('3840x2160')
  const [quality, setQuality] = useState('')
  const [count, setCount] = useState('1')
  const cells = props.table.size_tiers.flatMap((tier) =>
    props.table.qualities.map((q) => `${tier}_${q}`)
  )
  const complete = cells.every(
    (key) => prices[key]?.length && Number.isFinite(Number(prices[key]))
  )
  const dirty =
    props.isDraft ||
    cells.some((key) => prices[key] !== String(props.table.prices?.[key]))
  const generate = () =>
    imagePrices({
      profile: props.table.profile,
      prices: Object.fromEntries(
        cells.map((key) => [key, Number(prices[key])])
      ),
    })
  const apply = useMutation({ mutationFn: generate })
  const { onDirtyChange } = props
  useEffect(() => {
    onDirtyChange(dirty)
    return () => onDirtyChange(false)
  }, [dirty, onDirtyChange])
  useImperativeHandle(props.ref, () => ({
    commit: async () => {
      if (!dirty) return undefined
      if (!complete) {
        setMissingPrices(true)
        return null
      }
      try {
        const result = await apply.mutateAsync()
        return result.expression ?? null
      } catch {
        return null
      }
    },
  }))
  const [missingPrices, setMissingPrices] = useState(false)
  const preview = useMutation({
    mutationFn: async () => {
      const { expression } = await generate()
      if (!expression) throw new Error()
      const [result] = await previewBillingExpressions([
        {
          key: 'image',
          expression,
          sample: { body: { size, quality }, image_count: Number(count) },
        },
      ])
      if (!result?.evaluation) throw new Error(result?.error)
      return result.evaluation
    },
  })

  return (
    <div className='space-y-3'>
      <p className='text-muted-foreground text-xs'>
        {props.table.qualities.length === 1
          ? t('USD per image. Auto or omitted size uses 1K.')
          : t(
              'USD per image. Auto or omitted size uses 1K. Default quality is {{quality}}.',
              { quality: props.table.default_quality }
            )}{' '}
        {props.table.unmatched_uses_quality
          ? t('Unrecognized sizes use the 1K quality prices.')
          : t('Unrecognized sizes use the 1K default quality price.')}
      </p>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Size')}</TableHead>
            {props.table.qualities.map((q) => (
              <TableHead key={q}>{q === 'default' ? t('Price') : q}</TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.table.size_tiers.map((tier) => (
            <TableRow key={tier}>
              <TableHead scope='row'>{tier}</TableHead>
              {props.table.qualities.map((q) => {
                const key = `${tier}_${q}`
                return (
                  <TableCell key={key}>
                    <PricingAmountInput
                      aria-label={`${tier} ${q}`}
                      className='min-w-20'
                      value={prices[key] ?? ''}
                      disabled={apply.isPending || preview.isPending}
                      onChange={(value) => {
                        setPrices((current) => ({ ...current, [key]: value }))
                        preview.reset()
                      }}
                    />
                  </TableCell>
                )
              })}
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <div className='flex flex-wrap items-end gap-2'>
        <label className='space-y-1 text-sm'>
          {t('Size')}
          <Input
            value={size}
            onChange={(event) => {
              setSize(event.target.value)
              preview.reset()
            }}
          />
        </label>
        {props.table.qualities.length > 1 && (
          <label className='space-y-1 text-sm'>
            {t('Quality')}
            <Input
              value={quality}
              placeholder='auto'
              onChange={(event) => {
                setQuality(event.target.value)
                preview.reset()
              }}
            />
          </label>
        )}
        <label className='space-y-1 text-sm'>
          {t('Image count')}
          <Input
            type='number'
            min={1}
            step={1}
            value={count}
            onChange={(event) => {
              setCount(event.target.value)
              preview.reset()
            }}
          />
        </label>
        <Button
          type='button'
          size='sm'
          variant='outline'
          disabled={
            !complete ||
            !Number.isInteger(Number(count)) ||
            Number(count) < 1 ||
            preview.isPending ||
            apply.isPending
          }
          onClick={() => preview.mutate()}
        >
          {t('Preview')}
        </Button>
      </div>
      {preview.data && (
        <p role='status' className='text-sm'>
          {preview.data.matched_tier} · {preview.data.raw_cost_usd} USD
        </p>
      )}
      {missingPrices && !complete && (
        <p role='alert' className='text-destructive text-sm'>
          {t('Enter a price for every size and quality.')}
        </p>
      )}
      {(apply.isError || preview.isError) && (
        <p role='alert' className='text-destructive text-sm'>
          {t('Could not calculate image pricing.')}
        </p>
      )}
    </div>
  )
}
