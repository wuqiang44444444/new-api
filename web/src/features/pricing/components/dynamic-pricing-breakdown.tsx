/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { Tag as TagIcon } from 'lucide-react'
import { useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

import { ruleGroupsFromBillingDisplay } from '../lib/billing-display'
import {
  MATCH_CONTAINS,
  MATCH_EQ,
  MATCH_EXISTS,
  MATCH_GTE,
  MATCH_LT,
  MATCH_RANGE,
  SOURCE_TIME,
  requestRuleGroupsFromTrace,
  splitBillingExprAndRequestRules,
  tryParseRequestRuleExpr,
  type ParsedTaskTier,
  type RequestCondition,
  type RequestRuleGroup,
  type RequestRuleTrace,
} from '../lib/billing-expr'
import { isBreakdownTierMatched } from '../lib/breakdown-tier-match'
import {
  formatTaskUsageUnitPrice,
  type DynamicPriceLabelKind,
  type DynamicPriceOptions,
} from '../lib/dynamic-price'
import { getTaskPricingDisplayTiers } from '../lib/task-matrix-display'
import {
  taskPriceLabel,
  taskPricingConditionSummary,
} from '../lib/task-price-display'
import type {
  BillingDisplayProjection,
  BillingUsageSchema,
  BillingUsageUnit,
} from '../types'
import { TokenBillingBreakdown } from './token-billing-breakdown'

type DynamicPricingBreakdownProps = {
  billingExpr: string | null | undefined
  /**
   * Backend-generated strict projection of `billingExpr` for generic token
   * expressions. When absent or opaque, token prices are not rendered at all
   * (raw-expression fallback) instead of guessing them locally.
   */
  billingDisplay?: BillingDisplayProjection | null
  /**
   * Label of the tier that fired for the current request. When provided,
   * the corresponding row is highlighted and tagged as "Matched". Used by
   * the usage-log details dialog to show which tier the engine selected.
   */
  matchedTierLabel?: string | null
  /** Request-rule traces emitted by the settlement run. */
  requestRules?: RequestRuleTrace[] | null
  /**
   * Hide cache-pricing columns regardless of the tier values. The log
   * details dialog passes this when the actual request did not consume any
   * cache tokens, so users only see pricing rows that were relevant to the
   * call they are inspecting. Defaults to false (show all configured prices).
   */
  hideCacheColumns?: boolean
  /**
   * Dense rendering for the usage-log details dialog: drops the colored
   * icon header and uses the dialog's small text sizes. Defaults to false.
   */
  compact?: boolean
  usageSchema?: BillingUsageSchema
  /**
   * 投影驱动的任务档位。定价页面从后端任务 USD 投影取得；未提供时（日志
   * 详情等历史合同场景）沿用既有表达式解析展示，两者不互为回退。
   */
  tiers?: ParsedTaskTier[]
  taskPriceOptions?: Pick<
    DynamicPriceOptions,
    'showRechargePrice' | 'priceRate' | 'usdExchangeRate'
  >
  /**
   * Optional multiplier applied to displayed unit prices, e.g. a contract's
   * effective multiplier. When set, parsed tier unit prices are shown as the
   * effective contract price (base × multiplier); tier conditions and
   * thresholds stay untouched. Unparseable expressions fall back to the raw
   * expression display, which never applies the multiplier. Defaults to 1.
   */
  priceMultiplier?: number
  /**
   * Settlement usage facts from the consume log. Used to highlight the
   * expanded matrix display row when the engine label no longer matches
   * any synthesized combination label.
   */
  usageFacts?: Record<string, string | number>
}

type BreakdownPriceField = {
  id: string
  label: string
  labelKind: DynamicPriceLabelKind
  unit: BillingUsageUnit | 'request' | 'token'
  value: (tier: ParsedTaskTier) => number
}

function breakdownPriceFieldLabel(
  field: BreakdownPriceField,
  t: (key: string) => string
): ReactNode {
  if (field.labelKind === 'schema') {
    return <span className='break-words whitespace-normal'>{field.label}</span>
  }
  return t(field.label)
}

const TIME_FUNC_LABELS: Record<string, string> = {
  hour: 'Hour',
  minute: 'Minute',
  weekday: 'Weekday',
  month: 'Month',
  day: 'Day',
}

function formatBreakdownConditionSummary(
  tier: ParsedTaskTier,
  t: (key: string) => string,
  schema: BillingUsageSchema,
  language: string,
  tierCount: number
): string {
  return taskPricingConditionSummary(tier, schema, language, t, tierCount)
}

function formatBreakdownPrice(
  value: number,
  field: BreakdownPriceField,
  t: (key: string) => string,
  taskPriceOptions: DynamicPricingBreakdownProps['taskPriceOptions']
): string {
  const amount = formatTaskUsageUnitPrice(value, {
    tokenUnit: 'M',
    ...taskPriceOptions,
  })
  if (field.unit === 'second') return `${amount}/${t('s')}`
  if (field.unit === 'count') return `${amount}/${t('unit')}`
  if (field.unit === 'credit') return `${amount}/${t('credit')}`
  if (field.unit === 'token' && field.labelKind === 'schema') {
    return `${amount}/${t('1M token')}`
  }
  if (field.unit === 'request') return `${amount}/${t('request')}`
  return amount
}

function describeCondition(
  cond: RequestCondition,
  t: (key: string) => string
): string {
  if (cond.source === SOURCE_TIME) {
    const fn = t(TIME_FUNC_LABELS[cond.timeFunc] || cond.timeFunc)
    const tz = cond.timezone || 'UTC'
    if (cond.mode === MATCH_RANGE) {
      return `${fn} ${cond.rangeStart}:00~${cond.rangeEnd}:00 (${tz})`
    }
    const opMap: Record<string, string> = {
      [MATCH_EQ]: '=',
      [MATCH_GTE]: '≥',
      [MATCH_LT]: '<',
    }
    return `${fn} ${opMap[cond.mode] || '='} ${cond.value} (${tz})`
  }
  const src = cond.source === 'header' ? t('Header') : t('Body param')
  const path = cond.path || ''
  if (cond.mode === MATCH_EXISTS) return `${src} ${path} ${t('Exists')}`
  if (cond.mode === MATCH_CONTAINS) {
    return `${src} ${path} ${t('Contains')} "${cond.value}"`
  }
  const opMap: Record<string, string> = {
    eq: '=',
    gt: '>',
    gte: '≥',
    lt: '<',
    lte: '≤',
  }
  return `${src} ${path} ${opMap[cond.mode] || '='} ${cond.value}`
}

function describeGroup(
  group: RequestRuleGroup,
  t: (key: string) => string
): string {
  const description = (group.conditions || [])
    .map((condition) => describeCondition(condition, t))
    .join(' && ')
  return description || group.conditionText || ''
}

function nextOccurrenceKey(
  baseKey: string,
  occurrences: Map<string, number>
): string {
  const occurrence = occurrences.get(baseKey) || 0
  occurrences.set(baseKey, occurrence + 1)
  return `${baseKey}:${occurrence}`
}

export function DynamicPricingBreakdown(props: DynamicPricingBreakdownProps) {
  if (!props.billingExpr) return null
  if (props.usageSchema) {
    return <TaskPricingBreakdown {...props} usageSchema={props.usageSchema} />
  }
  const multiplier = props.priceMultiplier ?? 1
  return (
    <TokenBillingBreakdown
      expression={props.billingExpr}
      projection={props.billingDisplay}
      multiplier={
        Number.isFinite(multiplier) && multiplier >= 0 ? multiplier : 1
      }
      compact={props.compact ?? false}
      hideCacheColumns={props.hideCacheColumns ?? false}
      matchedTier={props.matchedTierLabel}
      traces={props.requestRules}
      priceOptions={props.taskPriceOptions}
    />
  )
}

function TaskPricingBreakdown({
  billingExpr,
  billingDisplay,
  matchedTierLabel,
  requestRules,
  compact = false,
  usageSchema,
  taskPriceOptions,
  priceMultiplier = 1,
  usageFacts,
  tiers: projectionTiers,
}: DynamicPricingBreakdownProps & { usageSchema: BillingUsageSchema }) {
  const { t, i18n } = useTranslation()
  const expr = billingExpr || ''
  const { tiers, ruleGroups } = useMemo(() => {
    if (projectionTiers) {
      // 定价页面：金额、条件与可证明倍率只来自后端投影，绝不回退本地解析。
      return {
        tiers: projectionTiers,
        ruleGroups:
          requestRules != null
            ? requestRuleGroupsFromTrace(requestRules)
            : ruleGroupsFromBillingDisplay(billingDisplay),
      }
    }
    const split = splitBillingExprAndRequestRules(expr)
    const parsedTiers = getTaskPricingDisplayTiers(
      split.billingExpr,
      usageSchema
    )
    const parsedRules =
      requestRules != null
        ? requestRuleGroupsFromTrace(requestRules)
        : tryParseRequestRuleExpr(split.requestRuleExpr || '')
    return {
      tiers: parsedTiers,
      ruleGroups: parsedRules || [],
    }
  }, [expr, usageSchema, requestRules, projectionTiers, billingDisplay])

  const hasTiers = tiers.length > 0
  const hasRules = ruleGroups.length > 0

  if (!expr) return null

  if (!hasTiers) {
    return (
      <section className={cn('min-w-0', !compact && 'py-4')}>
        {!compact && (
          <div className='mb-3 flex items-center gap-2'>
            <span className='inline-flex size-6 items-center justify-center rounded-lg bg-amber-100 text-amber-700 shadow-sm dark:bg-amber-500/20 dark:text-amber-300'>
              <TagIcon className='size-3.5' />
            </span>
            <div>
              <div className='text-foreground text-base font-medium'>
                {t('Special billing expression')}
              </div>
              <div className='text-muted-foreground text-xs'>
                {t('Unable to parse structured pricing')}
              </div>
            </div>
          </div>
        )}
        {projectionTiers ? (
          <div className='text-muted-foreground text-xs'>
            {t('Pricing details temporarily unavailable')}
          </div>
        ) : (
          <>
            <div className='text-muted-foreground mb-1 text-[10px] font-medium tracking-wider uppercase'>
              {t('Raw expression')}
            </div>
            <code className='text-muted-foreground block text-xs break-all'>
              {expr}
            </code>
          </>
        )}
      </section>
    )
  }

  const visiblePriceFields: BreakdownPriceField[] = Object.entries(usageSchema)
    .filter(
      ([field, definition]) =>
        definition.type === 'number' &&
        Boolean(definition.unit) &&
        // 显式零价也是有效报价：按字段存在性展示，缺失才省略。
        tiers.some(
          (tier) =>
            tier.unitPrices[field] != null &&
            Number.isFinite(Number(tier.unitPrices[field]))
        )
    )
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([field, definition]) => ({
      id: field,
      label: taskPriceLabel(definition.description, field, i18n.language),
      labelKind: 'schema',
      unit: definition.unit as BillingUsageUnit,
      value: (tier) => Number(tier.unitPrices[field] || 0),
    }))
  if (tiers.some((tier) => tier.hasConstant || tier.constant > 0)) {
    visiblePriceFields.push({
      id: 'constant',
      label: 'Additional charge',
      labelKind: 'i18n',
      unit: 'request',
      value: (tier) => tier.constant,
    })
  }
  const mobileTierKeyOccurrences = new Map<string, number>()
  const requestRuleKeyOccurrences = new Map<string, number>()

  const effectiveMultiplier =
    Number.isFinite(priceMultiplier) && priceMultiplier >= 0
      ? priceMultiplier
      : 1
  // 显式零价是有效报价：字段存在即渲染金额，只有缺失才显示“-”。
  const isFieldPresent = (tier: ParsedTaskTier, field: BreakdownPriceField) => {
    if (field.id === 'constant') {
      return tier.hasConstant || tier.constant !== 0
    }
    const raw = tier.unitPrices[field.id]
    return raw != null && Number.isFinite(Number(raw))
  }

  return (
    <section className={cn('min-w-0', !compact && 'py-3 sm:py-4')}>
      <div className={cn(compact ? cn(hasRules && 'mb-2') : 'mb-3 sm:mb-4')}>
        <div className='space-y-1.5 sm:hidden'>
          {tiers.map((tier) => {
            const condSummary = formatBreakdownConditionSummary(
              tier,
              t,
              usageSchema,
              i18n.language,
              tiers.length
            )
            const isMatched = isBreakdownTierMatched(
              tier,
              tiers,
              matchedTierLabel,
              usageFacts
            )
            const rowKey = nextOccurrenceKey(
              JSON.stringify(tier),
              mobileTierKeyOccurrences
            )
            return (
              <div
                key={`tier-mobile-${rowKey}`}
                className={cn(
                  'rounded-md border p-2',
                  isMatched && 'border-emerald-500/40 bg-emerald-500/10'
                )}
              >
                <div className='mb-1.5 flex flex-wrap items-center gap-1.5'>
                  {isMatched && (
                    <Badge
                      variant='secondary'
                      className='bg-emerald-100 text-emerald-700 dark:bg-emerald-500/20 dark:text-emerald-300'
                    >
                      {t('Matched')}
                    </Badge>
                  )}
                </div>
                {condSummary && (
                  <div className='text-muted-foreground mb-1.5 text-xs'>
                    {condSummary}
                  </div>
                )}
                <div
                  className={cn(
                    'grid gap-x-3 gap-y-1.5',
                    visiblePriceFields.length > 1 && 'grid-cols-2'
                  )}
                >
                  {visiblePriceFields.map((field) => (
                    <div key={field.id} className='min-w-0'>
                      <div className='text-muted-foreground text-xs font-medium break-words whitespace-normal'>
                        {breakdownPriceFieldLabel(field, t)}
                      </div>
                      <div
                        className={cn(
                          'break-words font-mono',
                          compact ? 'text-xs' : 'text-sm font-semibold'
                        )}
                      >
                        {isFieldPresent(tier, field)
                          ? formatBreakdownPrice(
                              field.value(tier) * effectiveMultiplier,
                              field,
                              t,
                              taskPriceOptions
                            )
                          : '-'}
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            )
          })}
        </div>
        <StaticDataTable
          className='hidden rounded-none border-0 sm:block'
          tableClassName={
            compact
              ? '[&_td]:text-xs [&_td_*]:text-xs [&_th]:text-xs [&_th_*]:text-xs'
              : 'text-sm'
          }
          headerRowClassName='hover:bg-transparent'
          data={tiers}
          getRowKey={(_tier, index) => `tier-${index}`}
          getRowClassName={(tier) => {
            const isMatched = isBreakdownTierMatched(
              tier,
              tiers,
              matchedTierLabel,
              usageFacts
            )
            return cn(
              isMatched &&
                'bg-emerald-50/70 hover:bg-emerald-50/70 dark:bg-emerald-500/10 dark:hover:bg-emerald-500/10'
            )
          }}
          columns={[
            {
              id: 'tier',
              header: t('Applicable conditions'),
              className: cn(
                'text-muted-foreground py-2 font-medium',
                compact && 'h-8'
              ),
              cellClassName: cn(
                'align-top whitespace-normal break-words',
                compact ? 'py-2' : 'py-2.5'
              ),
              cell: (tier) => {
                const condSummary = formatBreakdownConditionSummary(
                  tier,
                  t,
                  usageSchema,
                  i18n.language,
                  tiers.length
                )
                const isMatched = isBreakdownTierMatched(
                  tier,
                  tiers,
                  matchedTierLabel,
                  usageFacts
                )
                return (
                  <>
                    <div className='flex flex-wrap items-center gap-1.5'>
                      {isMatched && (
                        <Badge
                          variant='secondary'
                          className='bg-emerald-100 text-emerald-700 dark:bg-emerald-500/20 dark:text-emerald-300'
                        >
                          {t('Matched')}
                        </Badge>
                      )}
                    </div>
                    {condSummary && (
                      <div className='text-muted-foreground mt-1 text-xs'>
                        {condSummary}
                      </div>
                    )}
                  </>
                )
              },
            },
            ...visiblePriceFields.map((field) => ({
              id: field.id,
              header: breakdownPriceFieldLabel(field, t),
              className: cn(
                'text-muted-foreground py-2 text-right font-medium',
                compact && 'h-8'
              ),
              cellClassName: cn(
                'text-right align-top font-mono',
                compact ? 'py-2' : 'py-2.5'
              ),
              cell: (tier: ParsedTaskTier) => {
                if (!isFieldPresent(tier, field)) return '-'
                const value = field.value(tier) * effectiveMultiplier
                return (
                  <span className={cn(!compact && 'font-semibold')}>
                    {formatBreakdownPrice(value, field, t, taskPriceOptions)}
                  </span>
                )
              },
            })),
          ]}
        />
      </div>

      {hasRules && (
        <div>
          <div
            className={
              compact
                ? 'text-muted-foreground mb-1.5 text-xs font-medium'
                : 'text-foreground mb-2 text-sm font-semibold'
            }
          >
            {t('Conditional multipliers')}
          </div>
          <ul className='space-y-1.5'>
            {ruleGroups.map((group) => {
              const isMatched = group.matched === true
              const rowKey = nextOccurrenceKey(
                `${group.conditionText || JSON.stringify(group.conditions)}:${group.multiplier}`,
                requestRuleKeyOccurrences
              )
              return (
                <li
                  key={`group-${rowKey}`}
                  className={cn(
                    'bg-muted/50 flex items-center justify-between gap-3 rounded-md border border-transparent px-3 py-2',
                    isMatched && 'border-emerald-500/40 bg-emerald-500/10'
                  )}
                >
                  <span
                    className={cn(
                      'text-foreground break-all',
                      compact ? 'text-xs' : 'text-sm'
                    )}
                  >
                    {describeGroup(group, t)}
                  </span>
                  <Badge
                    variant='secondary'
                    className={cn(
                      'shrink-0 bg-orange-100 text-orange-700 dark:bg-orange-500/20 dark:text-orange-300',
                      isMatched &&
                        'bg-emerald-100 text-emerald-700 dark:bg-emerald-500/20 dark:text-emerald-300'
                    )}
                  >
                    {group.multiplier}x{isMatched && ` · ${t('Matched')}`}
                  </Badge>
                </li>
              )
            })}
          </ul>
        </div>
      )}
    </section>
  )
}
