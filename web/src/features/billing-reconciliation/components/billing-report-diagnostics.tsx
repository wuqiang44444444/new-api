import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { amountUnavailableReasonText } from '../estimate-display'
import type { resolveShanghaiMonth } from '../lib'

type BillingPeriod = ReturnType<typeof resolveShanghaiMonth>

type ReportDay = {
  date: string
  kind: 'usage' | 'billing'
  status: 'current' | 'rebuilding' | 'review' | 'coverage_issue'
  source?: 'frozen_details' | 'frozen_summary' | 'unavailable'
  published_source_rows?: number
  candidate_source_rows?: number
  issue_rows: number
}

type ReportLine = {
  source_id: number
  model: string
  net_quota: number
  previous_original_missing?: boolean
  previous_original?: string
  current_original?: string
  replay_issue?: string
  price_issue?: string
}

async function loadDiagnostics<T>(
  params: Record<string, string | number>
): Promise<T> {
  const { data } = await api.get('/api/billing/admin/reports/diagnostics', {
    params,
  })
  if (!data.success) throw new Error(data.message)
  return data.data
}

export function BillingReportDiagnostics(props: { period: BillingPeriod }) {
  const { t } = useTranslation()
  const root = useAuthStore(
    (state) => state.auth.user?.role === ROLE.SUPER_ADMIN
  )
  const [open, setOpen] = useState(false)
  const [date, setDate] = useState('')
  const days = useQuery({
    queryKey: [
      'daily-report-diagnostics',
      props.period.start_timestamp,
      props.period.end_timestamp,
    ],
    queryFn: () =>
      loadDiagnostics<ReportDay[]>({
        start_timestamp: props.period.start_timestamp,
        end_timestamp: props.period.end_timestamp,
      }),
    enabled: root && open,
    refetchInterval: 15_000,
  })
  if (!root) return null
  const statusLabels = {
    current: t('Published'),
    rebuilding: t('Rebuilding'),
    review: t('Needs review'),
    coverage_issue: t('Source coverage check failed'),
  }
  const sourceLabels = {
    frozen_details: t('Frozen line items'),
    frozen_summary: t('Frozen summaries'),
    unavailable: t('Historical source unavailable'),
  }

  return (
    <div className='min-w-0 space-y-2'>
      <Button
        variant='outline'
        size='sm'
        aria-expanded={open}
        onClick={() => setOpen(!open)}
      >
        {t('Daily report diagnostics')}
      </Button>
      {open && (
        <div className='max-h-80 overflow-auto rounded-md border p-2'>
          {days.isPending && <p role='status'>{t('Loading...')}</p>}
          {days.isError && (
            <p role='alert' className='text-destructive'>
              {days.error.message}
            </p>
          )}
          {days.data && (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Date')}</TableHead>
                  <TableHead>{t('Report')}</TableHead>
                  <TableHead>{t('Status')}</TableHead>
                  <TableHead>{t('Source')}</TableHead>
                  <TableHead>{t('Records to review')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {days.data.map((row) => (
                  <TableRow key={row.date + row.kind}>
                    <TableCell>{row.date}</TableCell>
                    <TableCell>
                      {row.kind === 'billing' ? t('Billing') : t('Usage')}
                    </TableCell>
                    <TableCell>{statusLabels[row.status]}</TableCell>
                    <TableCell>
                      {row.source && sourceLabels[row.source]}
                      {row.candidate_source_rows !== undefined &&
                        t(
                          'Published source rows: {{previous}} · Rebuild: {{current}}',
                          {
                            previous: row.published_source_rows,
                            current: row.candidate_source_rows,
                          }
                        )}
                    </TableCell>
                    <TableCell>
                      {row.issue_rows > 0 && (
                        <Button
                          variant='ghost'
                          size='sm'
                          aria-pressed={row.date === date}
                          onClick={() =>
                            setDate(row.date === date ? '' : row.date)
                          }
                        >
                          {row.issue_rows}
                        </Button>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
          {date && days.data?.some((day) => date === day.date) && (
            <DiagnosticLines key={date} date={date} />
          )}
        </div>
      )}
    </div>
  )
}

function DiagnosticLines(props: { date: string }) {
  const { t } = useTranslation()
  const [after, setAfter] = useState(0)
  const lines = useQuery({
    queryKey: ['daily-report-diagnostic-lines', props.date, after],
    queryFn: () => loadDiagnostics<ReportLine[]>({ date: props.date, after }),
  })

  return (
    <div className='border-t pt-2'>
      <p className='px-2 text-sm font-medium'>{props.date}</p>
      {lines.isPending && <p role='status'>{t('Loading...')}</p>}
      {lines.isError && (
        <p role='alert' className='text-destructive'>
          {lines.error.message}
        </p>
      )}
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Source ID')}</TableHead>
            <TableHead>{t('Model')}</TableHead>
            <TableHead>{t('Net quota')}</TableHead>
            <TableHead>{t('Previous original quota')}</TableHead>
            <TableHead>{t('Current original quota')}</TableHead>
            <TableHead>{t('Review reason')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {lines.data?.map((line) => (
            <TableRow key={line.source_id}>
              <TableCell>{line.source_id}</TableCell>
              <TableCell>{line.model}</TableCell>
              <TableCell>{line.net_quota}</TableCell>
              <TableCell>
                {line.previous_original_missing
                  ? t('Not recorded')
                  : line.previous_original || '—'}
              </TableCell>
              <TableCell>{line.current_original || '—'}</TableCell>
              <TableCell>
                {line.replay_issue && (
                  <p>{amountUnavailableReasonText([line.replay_issue], t)}</p>
                )}
                {line.price_issue === 'image_price_tier_mismatch' && (
                  <p>{t('Possible image pricing mismatch')}</p>
                )}
                {line.price_issue === 'image_price_inputs_missing' && (
                  <p>{t('Historical size or quality needs review')}</p>
                )}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      {after > 0 && (
        <Button size='sm' variant='ghost' onClick={() => setAfter(0)}>
          {t('First page')}
        </Button>
      )}
      {lines.data?.length === 100 && (
        <Button
          size='sm'
          variant='ghost'
          onClick={() => setAfter(lines.data?.at(-1)?.source_id ?? 0)}
        >
          {t('Next page')}
        </Button>
      )}
    </div>
  )
}
