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
import { useTranslation } from 'react-i18next'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { toIntlLocale } from '@/i18n/languages'
import { formatLogQuota } from '@/lib/format'

import { formatBillingDuration } from '../lib'
import type { BillingStatementRow } from '../lib/statement-rows'

type BillingStatementRowDetailsProps = {
  row: BillingStatementRow
  breakdownLoading: boolean
  breakdownUnavailable: boolean
}

function DetailMetric(props: { label: string; value: string }) {
  return (
    <div className='min-w-0'>
      <div className='text-muted-foreground text-xs'>{props.label}</div>
      <div className='mt-0.5 font-mono text-sm font-medium break-words tabular-nums'>
        {props.value}
      </div>
    </div>
  )
}

export function BillingStatementRowDetails(
  props: BillingStatementRowDetailsProps
) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage)
  const percentFormatter = new Intl.NumberFormat(locale, {
    style: 'percent',
    maximumFractionDigits: 1,
  })
  const number = (value: number) => value.toLocaleString(locale)

  return (
    <div className='grid gap-2 p-3 lg:grid-cols-3'>
      <Card size='sm'>
        <CardHeader>
          <CardTitle>{t('Usage composition')}</CardTitle>
        </CardHeader>
        <CardContent className='grid grid-cols-2 gap-3'>
          <DetailMetric
            label={t('Input Tokens')}
            value={number(props.row.promptTokens)}
          />
          <DetailMetric
            label={t('Output Tokens')}
            value={number(props.row.completionTokens)}
          />
          <DetailMetric
            label={t('Average Response Time')}
            value={formatBillingDuration(
              props.row.averageUseTimeSeconds,
              locale
            )}
          />
          <DetailMetric
            label={t('Streaming Requests')}
            value={number(props.row.streamRequests)}
          />
          {props.breakdownLoading && (
            <div className='col-span-2 space-y-2' aria-label={t('Loading')}>
              <Skeleton className='h-4 w-full' />
              <Skeleton className='h-4 w-3/4' />
            </div>
          )}
          {!props.breakdownLoading &&
            props.row.breakdownComplete &&
            props.row.cache && (
              <>
                <DetailMetric
                  label={t('Cache Read Tokens')}
                  value={number(props.row.cache.read_tokens)}
                />
                <DetailMetric
                  label={t('Cache Write Tokens')}
                  value={number(props.row.cache.write_tokens)}
                />
                {props.row.cache.write_tokens_5m > 0 && (
                  <DetailMetric
                    label={t('Cache Write Tokens (5m)')}
                    value={number(props.row.cache.write_tokens_5m)}
                  />
                )}
                {props.row.cache.write_tokens_1h > 0 && (
                  <DetailMetric
                    label={t('Cache Write Tokens (1h)')}
                    value={number(props.row.cache.write_tokens_1h)}
                  />
                )}
                <DetailMetric
                  label={t('Cache Hit Requests')}
                  value={`${number(props.row.cache.hit_requests)} / ${number(props.row.cache.denominator_requests)}`}
                />
                <DetailMetric
                  label={t('Request Cache Hit Rate')}
                  value={percentFormatter.format(
                    props.row.cache.hit_request_ratio
                  )}
                />
                <p className='text-muted-foreground col-span-2 text-xs'>
                  {t(
                    'The cache hit denominator is all settled requests in this row.'
                  )}
                </p>
              </>
            )}
          {!props.breakdownLoading &&
            !props.breakdownUnavailable &&
            props.row.breakdownComplete &&
            !props.row.cache && (
              <p className='text-muted-foreground col-span-2 text-xs'>
                {t('No cache activity was recorded for this row.')}
              </p>
            )}
          {!props.breakdownLoading &&
            !props.breakdownUnavailable &&
            props.row.breakdownComplete && (
              <p className='text-muted-foreground col-span-2 text-xs'>
                {t(
                  'Historical logs do not reliably separate ordinary input from cache read and write tokens.'
                )}
              </p>
            )}
          {props.breakdownUnavailable && (
            <p className='text-destructive col-span-2 text-xs'>
              {t('Usage composition is temporarily unavailable.')}
            </p>
          )}
          {!props.breakdownLoading &&
            !props.breakdownUnavailable &&
            !props.row.breakdownComplete && (
              <p className='text-destructive col-span-2 text-xs'>
                {t('Usage composition is unavailable for this historical row.')}
              </p>
            )}
        </CardContent>
      </Card>

      <Card size='sm'>
        <CardHeader>
          <CardTitle>{t('Context classification')}</CardTitle>
        </CardHeader>
        <CardContent className='grid grid-cols-2 gap-3'>
          {props.breakdownLoading && (
            <div className='col-span-2 space-y-2' aria-label={t('Loading')}>
              <Skeleton className='h-4 w-full' />
              <Skeleton className='h-4 w-3/4' />
            </div>
          )}
          {!props.breakdownLoading &&
            props.row.breakdownComplete &&
            props.row.context && (
              <>
                <DetailMetric
                  label={t('Current Context Threshold')}
                  value={
                    props.row.context.threshold_tokens == null
                      ? t('Multiple model thresholds')
                      : number(props.row.context.threshold_tokens)
                  }
                />
                <DetailMetric
                  label={t('Classification Coverage')}
                  value={`${percentFormatter.format(
                    props.row.context.classification_coverage
                  )} (${number(props.row.context.classified_requests)} / ${number(
                    props.row.context.classified_requests +
                      props.row.context.unclassified_requests
                  )})`}
                />
                <DetailMetric
                  label={t('Short Context Requests')}
                  value={number(props.row.context.short_requests)}
                />
                <DetailMetric
                  label={t('Long Context Requests')}
                  value={number(props.row.context.long_requests)}
                />
                <DetailMetric
                  label={t('Short Context Input Tokens')}
                  value={number(props.row.context.short_input_tokens)}
                />
                <DetailMetric
                  label={t('Long Context Input Tokens')}
                  value={number(props.row.context.long_input_tokens)}
                />
                <DetailMetric
                  label={t('Short Context Request Cost')}
                  value={formatLogQuota(props.row.context.short_gross_quota)}
                />
                <DetailMetric
                  label={t('Long Context Request Cost')}
                  value={formatLogQuota(props.row.context.long_gross_quota)}
                />
                <DetailMetric
                  label={t('Unclassified Requests')}
                  value={number(props.row.context.unclassified_requests)}
                />
                <p className='text-muted-foreground col-span-2 text-xs'>
                  {t(
                    'Historical requests are classified using the current model threshold and may change when that setting changes.'
                  )}
                </p>
              </>
            )}
          {!props.breakdownLoading &&
            !props.breakdownUnavailable &&
            props.row.breakdownComplete &&
            !props.row.context && (
              <p className='text-muted-foreground col-span-2 text-xs'>
                {t('No Context threshold is configured for this row.')}
              </p>
            )}
          {props.breakdownUnavailable && (
            <p className='text-destructive col-span-2 text-xs'>
              {t('Context classification is temporarily unavailable.')}
            </p>
          )}
          {!props.breakdownLoading &&
            !props.breakdownUnavailable &&
            !props.row.breakdownComplete && (
              <p className='text-destructive col-span-2 text-xs'>
                {t(
                  'Context classification is unavailable for this historical row.'
                )}
              </p>
            )}
        </CardContent>
      </Card>

      <Card size='sm'>
        <CardHeader>
          <CardTitle>{t('Billing and settlement')}</CardTitle>
        </CardHeader>
        <CardContent className='grid grid-cols-2 gap-3'>
          <DetailMetric
            label={t('Gross Usage Cost')}
            value={formatLogQuota(props.row.grossQuota)}
          />
          <DetailMetric
            label={t('Refunds / Adjustments')}
            value={formatLogQuota(props.row.refundQuota)}
          />
          <DetailMetric
            label={t('Net Settled Cost')}
            value={formatLogQuota(props.row.netQuota)}
          />
          {props.row.unallocatedAdjustmentQuota > 0 && (
            <>
              <DetailMetric
                label={t('Unallocated Async Adjustments')}
                value={formatLogQuota(props.row.unallocatedAdjustmentQuota)}
              />
              <p className='text-muted-foreground col-span-2 text-xs'>
                {t(
                  'Async adjustments are included in gross usage cost but are not assigned to cache, Context, or dynamic billing categories.'
                )}
              </p>
            </>
          )}
          {props.row.billingMode && (
            <>
              <DetailMetric
                label={t('Dynamic Billing Requests')}
                value={number(props.row.billingMode.tiered_requests)}
              />
              <DetailMetric
                label={t('Dynamic Billing Request Cost')}
                value={formatLogQuota(props.row.billingMode.tiered_gross_quota)}
              />
            </>
          )}
          {props.row.cache && (
            <>
              <DetailMetric
                label={t('Settled Cost of Cache-hit Requests')}
                value={formatLogQuota(props.row.cache.hit_request_gross_quota)}
              />
              <p className='text-muted-foreground col-span-2 text-xs'>
                {t(
                  'Cache-hit request cost is the settled base consume-log amount, not cache cost or savings.'
                )}
              </p>
            </>
          )}
          {props.row.statementSaturated && (
            <p className='text-destructive col-span-2 text-xs'>
              <span className='font-medium'>{t('Reporting Limit Reached')}</span>
              {' — '}
              {t(
                'One or more statement totals reached the reporting limit. Review usage logs before reconciliation.'
              )}
            </p>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
