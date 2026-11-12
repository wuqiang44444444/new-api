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
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from '@/components/ui/drawer'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { BillingFundGroupTable } from '@/features/usage-logs/components/billing-fund-groups'
import { LogDiscountCell } from '@/features/usage-logs/components/log-discount-cell'
import type { LogOtherData } from '@/features/usage-logs/types'

import {
  getAdminVersionLines,
  getSelfVersionLines,
  type BillingStatementVersionInfo,
  type BillingStatementVersionLine,
} from '../version-api'
import { formatVersionQuota } from '../version-money'
import { BillingPagination } from './billing-pagination'

type VersionLinesDrawerProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  version: BillingStatementVersionInfo | null
  isAdmin: boolean
  /** 由模型行带入的初始筛选（仅 api_key 维度有 token_id） */
  initialFilter?: {
    channel_id?: number
    token_id?: number
    model_name?: string
    billing_mode?: string
  }
}

// 版本明细抽屉（方案 13）：已确认/待确认版本的下钻只读版本明细，不扫描原始日志。
export function VersionLinesDrawer(props: VersionLinesDrawerProps) {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(50)
  const [fundGroup, setFundGroup] = useState<string>()
  const [filter, setFilter] = useState(props.initialFilter ?? {})

  const version = props.version
  const linesQuery = useQuery({
    queryKey: [
      'billing-statement-version-lines',
      props.isAdmin,
      version?.draft_public_id,
      page,
      pageSize,
      filter,
      fundGroup,
    ],
    queryFn: async ({ signal }) => {
      if (!version) {
        throw new Error('no version')
      }
      const params = {
        ...filter,
        page,
        page_size: pageSize,
        group_funds: !fundGroup,
        fund_group: fundGroup,
      }
      const response = props.isAdmin
        ? await getAdminVersionLines(version.draft_public_id, params, signal)
        : await getSelfVersionLines(version.draft_public_id, params, signal)
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Unable to load version lines.'))
      }
      return response.data
    },
    enabled: props.open && version != null,
  })

  const total = linesQuery.data?.total ?? 0

  return (
    <Drawer
      open={props.open}
      onOpenChange={(open) => {
        if (!open) {
          setPage(1)
          setFilter({})
          setFundGroup(undefined)
        }
        props.onOpenChange(open)
      }}
    >
      <DrawerContent className='max-h-[85vh]'>
        <DrawerHeader>
          <DrawerTitle>
            {t('Version details')}
            {version?.version_number != null && (
              <Badge variant='secondary' className='ms-2'>
                v{version.version_number}
              </Badge>
            )}
          </DrawerTitle>
          <DrawerDescription>
            {t(
              'Frozen statement lines. Source logs may have changed after confirmation.'
            )}
            {filter.model_name && (
              <Button
                variant='link'
                size='xs'
                onClick={() => {
                  setFilter({})
                  setFundGroup(undefined)
                  setPage(1)
                }}
              >
                {t('Clear filter: {{model}}', { model: filter.model_name })}
              </Button>
            )}
          </DrawerDescription>
        </DrawerHeader>
        <div className='overflow-y-auto px-4 pb-4'>
          {linesQuery.isError && <p role='alert'>{linesQuery.error.message}</p>}
          {fundGroup ? (
            <>
              <Button
                variant='outline'
                size='sm'
                onClick={() => {
                  setFundGroup(undefined)
                  setPage(1)
                }}
              >
                {t('Back')}
              </Button>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('Time')}</TableHead>
                    <TableHead>{t('Type')}</TableHead>
                    <TableHead>{t('API Key')}</TableHead>
                    <TableHead>{t('Model')}</TableHead>
                    <TableHead>{t('Discount')}</TableHead>
                    <TableHead className='text-right'>{t('Input')}</TableHead>
                    <TableHead className='text-right'>{t('Output')}</TableHead>
                    <TableHead className='text-right'>
                      {t('Net amount')}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {(linesQuery.data?.lines ?? []).map((line) => (
                    <TableRow key={line.id}>
                      <TableCell className='whitespace-nowrap'>
                        {new Date(line.created_at * 1000).toLocaleString()}
                      </TableCell>
                      <TableCell>
                        <Badge
                          variant={
                            line.log_type === 6 ? 'secondary' : 'outline'
                          }
                        >
                          {line.log_type === 6 ? t('Refund') : t('Consume')}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        {line.token_name ||
                          t('API Key #{{id}}', { id: line.token_id })}
                      </TableCell>
                      <TableCell>
                        {line.customer_model || t('Unknown model')}
                      </TableCell>
                      <TableCell>
                        <LogDiscountCell
                          t={t}
                          other={{
                            group_ratio: line.facts.group_ratio,
                            contract_name: line.facts.contract_name,
                            contract_discount: line.facts.contract_ratio,
                            contract_applicable:
                              line.facts.contract_applicable === 'unknown'
                                ? undefined
                                : line.facts.contract_applicable === 'yes',
                          }}
                        />
                      </TableCell>
                      <TableCell className='text-right tabular-nums'>
                        {line.facts.input_tokens_unavailable
                          ? t('Unknown')
                          : line.input_tokens}
                      </TableCell>
                      <TableCell className='text-right tabular-nums'>
                        {line.output_tokens}
                      </TableCell>
                      <TableCell className='text-right tabular-nums'>
                        {formatVersionQuota(
                          line.log_type === 6 ? -line.quota : line.quota,
                          version
                        )}
                        <FrozenExplanation line={line} />
                      </TableCell>
                    </TableRow>
                  ))}
                  {linesQuery.isSuccess && total === 0 && (
                    <TableRow>
                      <TableCell colSpan={8} className='text-center'>
                        {t('No settled usage in this billing period.')}
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </>
          ) : (
            <BillingFundGroupTable
              rows={linesQuery.data?.fund_groups ?? []}
              loading={linesQuery.isPending}
              formatAmount={(quota) => formatVersionQuota(quota, version)}
              onSelect={(group) => {
                setFundGroup(group)
                setPage(1)
              }}
            />
          )}

          {linesQuery.isSuccess && total > 0 && (
            <BillingPagination
              page={page}
              pageSize={pageSize}
              total={total}
              label={t('Details')}
              disabled={linesQuery.isFetching}
              onChange={(nextPage, nextSize) => {
                setPage(nextPage)
                setPageSize(nextSize)
              }}
            />
          )}
        </div>
      </DrawerContent>
    </Drawer>
  )
}

function FrozenExplanation(props: { line: BillingStatementVersionLine }) {
  const { t } = useTranslation()
  const facts = props.line.facts
  if (facts.explanation_status !== 'available' || !facts.billing_line_items) {
    return (
      <div className='text-muted-foreground text-xs'>{t('Unavailable')}</div>
    )
  }
  const lines = JSON.parse(facts.billing_line_items) as NonNullable<
    LogOtherData['billing_explanation']
  >['lines']
  return (
    <div className='text-muted-foreground text-xs'>
      {lines.map((line) => (
        <div key={`${line.label}:${line.unit}`}>
          {t(line.label)}: {line.quantity.toLocaleString()} {t(line.unit)} × $
          {line.unit_price_usd}/{line.unit === 'token' ? 'M' : t(line.unit)} = $
          {line.subtotal_usd.toFixed(8)}
        </div>
      ))}
    </div>
  )
}
