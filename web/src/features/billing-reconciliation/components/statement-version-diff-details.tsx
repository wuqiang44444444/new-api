import { useTranslation } from 'react-i18next'

import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import { billingModeLabel } from '../lib'
import type { BillingMode } from '../types'
import type { BillingStatementVersionDiff } from '../version-api'

// Frozen counts remain decimal strings; unknown usage must not become zero.
export function StatementVersionDiffDetails(props: {
  diff: BillingStatementVersionDiff
}) {
  const { t } = useTranslation()
  const labels: Record<string, string> = {
    input_tokens: t('Input tokens'),
    output_tokens: t('Output tokens'),
    cache_read_tokens: t('Cache read tokens'),
    cache_write_tokens: t('Cache write tokens'),
    billable_calls: t('Billable calls'),
    refunded_calls: t('Refunded calls'),
    input_tokens_unavailable_requests: t('Requests with unknown input usage'),
    unknown_billing_mode_requests: t('Requests with unknown billing mode'),
    unavailable_requests: t('Requests with incomplete evidence'),
    cache_write_unavailable_requests: t('Requests with unknown cache writes'),
    missing_historical_price_rows: t('Rows with missing historical prices'),
    provider_model_fallback_rows: t('Rows using provider model fallback'),
  }
  const ratioSources: Record<string, string> = {
    group: t('Group ratio'),
    user_exclusive: t('User exclusive ratio'),
  }
  return (
    <div className='space-y-4'>
      {[
        { title: t('Usage changes'), rows: props.diff.usage },
        { title: t('Quality changes'), rows: props.diff.quality },
      ].map((section) => (
        <Table key={section.title} aria-label={section.title}>
          <TableHeader>
            <TableRow>
              <TableHead>{section.title}</TableHead>
              <TableHead>{t('Confirmed')}</TableHead>
              <TableHead>{t('Correction draft')}</TableHead>
              <TableHead>{t('Difference')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {Object.entries(section.rows ?? {}).map(([key, item]) => (
              <TableRow key={key}>
                <TableCell>{labels[key] ?? key}</TableCell>
                <TableCell>{item.base ?? t('Unavailable')}</TableCell>
                <TableCell>{item.compare ?? t('Unavailable')}</TableCell>
                <TableCell>{item.delta ?? '—'}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      ))}
      {[
        {
          title: t('Discount combinations before correction'),
          rows: props.diff.discounts?.base,
        },
        {
          title: t('Discount combinations after correction'),
          rows: props.diff.discounts?.compare,
        },
      ].map((section) => (
        <Table key={section.title} aria-label={section.title}>
          <TableHeader>
            <TableRow>
              <TableHead>{section.title}</TableHead>
              <TableHead>{t('Model')}</TableHead>
              <TableHead>{t('Group ratio')}</TableHead>
              <TableHead>{t('Contract')}</TableHead>
              <TableHead>{t('Contract discount')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {(section.rows ?? []).map((row) => {
              const key = JSON.stringify([
                row.group_id,
                row.model_name,
                row.billing_mode,
                row.group_name,
                row.group_ratio_source,
                row.group_ratio,
                row.contract_applicable,
                row.contract_id_known,
                row.contract_id,
                row.contract_name,
                row.contract_version,
                row.contract_ratio,
                row.other,
              ])
              let contract = t('Unknown')
              if (row.contract_applicable === 'unrecorded') {
                contract = t('Not recorded')
              }
              if (row.contract_applicable === 'no') {
                contract = t('Not applicable')
              }
              if (row.contract_applicable === 'yes') {
                contract = `${row.contract_name || t('Unknown')} / ${row.contract_version ?? t('Unknown')}`
                if (row.contract_id_known) {
                  contract += ` (#${row.contract_id})`
                }
              }
              return (
                <TableRow key={key}>
                  <TableCell>{row.group_name || t('Unknown')}</TableCell>
                  <TableCell>
                    {row.model_name} (
                    {t(billingModeLabel(row.billing_mode as BillingMode))})
                  </TableCell>
                  <TableCell>
                    {row.group_ratio ?? t('Unavailable')} (
                    {ratioSources[row.group_ratio_source ?? ''] ?? t('Unknown')}
                    )
                  </TableCell>
                  <TableCell>{contract}</TableCell>
                  <TableCell>
                    {row.contract_ratio != null ? row.contract_ratio : contract}
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      ))}
    </div>
  )
}
