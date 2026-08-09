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
import {
  ArrowLeft01Icon,
  ArrowDown01Icon,
  ArrowRight01Icon,
  LinkSquare02Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatQuotaWithCurrency } from '@/lib/currency'

import { getAdminCustomerStatement, getSelfCustomerStatement } from '../api'
import { billingModeLabel, formatInteger } from '../lib'
import type { BillingDimension, CustomerModelSummary } from '../types'

type CustomerStatementProps = {
  isAdmin: boolean
  period: { start_timestamp: number; end_timestamp: number }
  userId?: number
  dimension: BillingDimension
  onBack?: () => void
  onDimensionChange: (dimension: BillingDimension) => void
}

export function CustomerStatementView(props: CustomerStatementProps) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState<Set<number>>(() => new Set())
  const selectedUserId = props.userId
  const statementQuery = useQuery({
    queryKey: [
      'billing-customer-reconciliation',
      props.isAdmin,
      selectedUserId,
      props.dimension,
      props.period.start_timestamp,
      props.period.end_timestamp,
    ],
    queryFn: async () => {
      if (props.isAdmin && selectedUserId == null) {
        throw new Error(t('Select customer'))
      }
      const response = props.isAdmin
        ? await getAdminCustomerStatement({
            ...props.period,
            user_id: selectedUserId as number,
            dimension: props.dimension,
          })
        : await getSelfCustomerStatement(props.period)
      if (!response.success || !response.data) {
        throw new Error(
          response.message || t('Unable to load customer billing.')
        )
      }
      return response.data
    },
    enabled: !props.isAdmin || selectedUserId != null,
    staleTime: 30_000,
    retry: false,
  })

  const statement = statementQuery.data?.result
  const groups = statement?.groups ?? []
  if (statementQuery.isError) {
    return (
      <ErrorState
        title={t('Unable to load customer billing')}
        description={
          statementQuery.error instanceof Error
            ? statementQuery.error.message
            : t('Please try again later.')
        }
        onRetry={() => statementQuery.refetch()}
      />
    )
  }

  return (
    <div className='space-y-3'>
      {props.isAdmin && props.onBack && (
        <Button variant='ghost' size='sm' onClick={props.onBack}>
          <HugeiconsIcon
            icon={ArrowLeft01Icon}
            strokeWidth={2}
            data-icon='inline-start'
          />
          {t('Back to customer statements')}
        </Button>
      )}
      <Card size='sm'>
        <CardContent className='grid gap-3 md:grid-cols-3'>
          {props.isAdmin && statement && (
            <div className='space-y-1.5'>
              <div className='text-muted-foreground text-xs font-medium'>
                {t('Customer')}
              </div>
              <div className='font-medium'>
                {customerStatementLabel(statement)}
              </div>
              <div className='text-muted-foreground text-xs'>
                {t('Customer ID')}: {statement.user_id}
              </div>
            </div>
          )}
          {props.isAdmin && (
            <label className='space-y-1.5'>
              <span className='text-muted-foreground text-xs font-medium'>
                {t('Aggregation dimension')}
              </span>
              <Select
                items={[
                  { value: 'api_key', label: t('API Key') },
                  { value: 'channel', label: t('Channel') },
                ]}
                value={props.dimension}
                onValueChange={(value) =>
                  value != null &&
                  props.onDimensionChange(value as BillingDimension)
                }
              >
                <SelectTrigger className='w-full'>
                  <SelectValue>
                    {props.dimension === 'api_key'
                      ? t('API Key')
                      : t('Channel')}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent align='start'>
                  <SelectGroup>
                    <SelectItem value='api_key'>{t('API Key')}</SelectItem>
                    <SelectItem value='channel'>{t('Channel')}</SelectItem>
                  </SelectGroup>
                </SelectContent>
              </Select>
            </label>
          )}
          <div className='space-y-1.5 md:self-end'>
            <div className='text-muted-foreground text-xs font-medium'>
              {t('Authoritative source')}
            </div>
            <div className='flex h-8 items-center gap-2'>
              <Badge variant='outline'>{t('Database')}</Badge>
              <span className='text-muted-foreground text-xs'>
                {t('Asia/Shanghai settlement period')}
              </span>
            </div>
          </div>
        </CardContent>
      </Card>

      {statementQuery.isPending || !statement ? (
        <StatementSkeleton />
      ) : (
        <>
          {statement.data_quality?.status === 'partial' && (
            <div className='border-warning/40 bg-warning/10 text-warning rounded-lg border px-3 py-2 text-sm'>
              {t(
                'Some usage details are unavailable or no longer match the settled statement snapshot. Refresh before exporting.'
              )}
            </div>
          )}
          <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
            <SummaryCard
              title={t('Current balance')}
              value={formatQuotaWithCurrency(statement.current_balance)}
              description={t('Read directly from the main database')}
            />
            <SummaryCard
              title={t('Original amount')}
              value={formatQuotaWithCurrency(statement.original_quota)}
              description={t(
                'Unavailable when historical price snapshots are missing'
              )}
            />
            <SummaryCard
              title={t('Discount savings')}
              value={formatQuotaWithCurrency(statement.discount_quota)}
              description={t('Original amount minus settled gross amount')}
            />
            <SummaryCard
              title={t('Net settled amount')}
              value={formatQuotaWithCurrency(statement.summary.net_quota)}
              description={t('{{count}} aggregated requests', {
                count: statement.summary.requests,
              })}
            />
          </div>

          <Card>
            <CardHeader>
              <CardTitle>{t('Customer billing summary')}</CardTitle>
              <CardDescription>
                {t(
                  'Expand a group to inspect model totals. Request rows remain in usage logs.'
                )}
              </CardDescription>
            </CardHeader>
            <CardContent className='px-0'>
              {groups.length === 0 ? (
                <div className='text-muted-foreground px-4 py-12 text-center'>
                  {t('No settled usage in this billing period.')}
                </div>
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead className='w-10' />
                      <TableHead>
                        {props.dimension === 'api_key'
                          ? t('API Key')
                          : t('Channel')}
                      </TableHead>
                      <TableHead className='text-right'>
                        {t('Requests')}
                      </TableHead>
                      <TableHead className='text-right'>
                        {t('Original amount')}
                      </TableHead>
                      <TableHead className='text-right'>
                        {t('Discount')}
                      </TableHead>
                      <TableHead className='text-right'>
                        {t('Net amount')}
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {groups.map((group) => {
                      const isExpanded = expanded.has(group.id)
                      return [
                        <TableRow key={`group-${group.id}`}>
                          <TableCell>
                            <Button
                              variant='ghost'
                              size='icon-sm'
                              aria-label={
                                isExpanded
                                  ? t('Collapse models')
                                  : t('Expand models')
                              }
                              aria-expanded={isExpanded}
                              onClick={() =>
                                setExpanded((current) =>
                                  toggleSet(current, group.id)
                                )
                              }
                            >
                              <HugeiconsIcon
                                icon={
                                  isExpanded
                                    ? ArrowDown01Icon
                                    : ArrowRight01Icon
                                }
                                strokeWidth={2}
                              />
                            </Button>
                          </TableCell>
                          <TableCell>
                            <div className='font-medium'>{group.name}</div>
                            <div className='text-muted-foreground text-xs'>
                              #{group.id}
                            </div>
                          </TableCell>
                          <TableCell className='text-right'>
                            {formatInteger(group.usage.requests)}
                          </TableCell>
                          <TableCell className='text-right'>
                            {formatQuotaWithCurrency(group.original_quota)}
                          </TableCell>
                          <TableCell className='text-right'>
                            {formatQuotaWithCurrency(group.discount_quota)}
                          </TableCell>
                          <TableCell className='text-right font-medium'>
                            {formatQuotaWithCurrency(group.usage.net_quota)}
                          </TableCell>
                        </TableRow>,
                        ...(isExpanded
                          ? group.models.map((model) => (
                              <CustomerModelRow
                                key={`model-${group.id}-${model.model_name}-${model.billing_mode}`}
                                model={model}
                                dimension={props.dimension}
                                username={statement.username}
                              />
                            ))
                          : []),
                      ]
                    })}
                  </TableBody>
                </Table>
              )}
            </CardContent>
          </Card>
        </>
      )}
    </div>
  )
}

function CustomerModelRow(props: {
  model: CustomerModelSummary
  dimension: BillingDimension
  username?: string
}) {
  const { t } = useTranslation()
  const filter = props.model.detail_filter
  const params = new URLSearchParams({
    model: props.model.model_name,
    startTime: String(filter.start_timestamp * 1000),
    endTime: String(filter.end_timestamp * 1000),
  })
  if (props.username) params.set('username', props.username)
  if (props.dimension === 'channel') {
    params.set('channel', String(filter.channel_id ?? ''))
  }
  if (props.dimension === 'api_key' && filter.token_id) {
    params.set('tokenId', String(filter.token_id))
  }
  const detailHref = `/usage-logs/common?${params.toString()}`
  const usage = props.model.usage
  return (
    <TableRow className='bg-muted/20'>
      <TableCell />
      <TableCell>
        <div className='pl-4'>
          <div className='flex items-center gap-2'>
            <span>{props.model.model_name || t('Unknown model')}</span>
            <Badge variant='secondary'>
              {t(billingModeLabel(props.model.billing_mode))}
            </Badge>
          </div>
          <div className='text-muted-foreground mt-1 text-xs'>
            {props.model.billing_mode === 'token'
              ? t(
                  'Input {{input}} · Cache read {{cacheRead}} · Cache write {{cacheWrite}} · Output {{output}}',
                  {
                    input: formatInteger(usage.input_tokens),
                    cacheRead: formatInteger(usage.cache_read_tokens),
                    cacheWrite: formatInteger(usage.cache_write_tokens),
                    output: formatInteger(usage.output_tokens),
                  }
                )
              : t('Billable {{billable}} · Refunded {{refunded}}', {
                  billable: formatInteger(usage.billable_calls),
                  refunded: formatInteger(usage.refunded_calls),
                })}
          </div>
        </div>
      </TableCell>
      <TableCell className='text-right'>
        {formatInteger(usage.requests)}
      </TableCell>
      <TableCell className='text-right'>
        {formatQuotaWithCurrency(props.model.original_quota)}
      </TableCell>
      <TableCell className='text-right'>
        {customerDiscountLabel(props.model, t)}
      </TableCell>
      <TableCell className='text-right'>
        <div className='font-medium'>
          {formatQuotaWithCurrency(usage.net_quota)}
        </div>
        <Button
          variant='link'
          size='xs'
          nativeButton={false}
          render={<a href={detailHref} />}
        >
          {t('View details')}
          <HugeiconsIcon
            icon={LinkSquare02Icon}
            strokeWidth={2}
            data-icon='inline-end'
          />
        </Button>
      </TableCell>
    </TableRow>
  )
}

function customerDiscountLabel(
  model: CustomerModelSummary,
  translate: (key: string) => string
) {
  if (model.multiple_discounts) {
    return translate('Multiple versions')
  }
  if (model.discount_ratio == null) {
    return translate('Unknown')
  }
  return model.discount_ratio.toFixed(4)
}

function SummaryCard(props: {
  title: string
  value: string
  description: string
}) {
  return (
    <Card size='sm'>
      <CardHeader>
        <CardDescription>{props.title}</CardDescription>
        <CardTitle className='text-xl tabular-nums'>{props.value}</CardTitle>
      </CardHeader>
      <CardContent className='text-muted-foreground text-xs'>
        {props.description}
      </CardContent>
    </Card>
  )
}

function StatementSkeleton() {
  return (
    <div className='space-y-3'>
      <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
        {[0, 1, 2, 3].map((item) => (
          <Skeleton key={item} className='h-28 rounded-xl' />
        ))}
      </div>
      <Skeleton className='h-72 rounded-xl' />
    </div>
  )
}

function customerStatementLabel(statement: {
  username: string
  display_name: string
}) {
  const displayName = statement.display_name?.trim()
  return displayName
    ? `${displayName} (${statement.username})`
    : statement.username
}

function toggleSet(current: Set<number>, value: number) {
  const next = new Set(current)
  if (next.has(value)) next.delete(value)
  else next.add(value)
  return next
}
