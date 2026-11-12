import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { buttonVariants } from '@/components/ui/button'

import type { UsageAnalyticsPeriod } from './types'
import { formatPeriodRange } from './utils'

export function UsageReportEmpty(props: {
  period: UsageAnalyticsPeriod
  personal?: boolean
}) {
  const { t } = useTranslation()
  const period = props.period
  const action = props.personal ? (
    <Link
      className={buttonVariants({ variant: 'outline' })}
      to='/usage-logs/$section'
      params={{ section: 'common' }}
      search={{
        scope: 'self',
        startTime: period.start_timestamp * 1000,
        // The report end is exclusive and the log filter includes its final second.
        endTime: (period.end_timestamp - 1) * 1000,
      }}
    >
      {t('View my usage logs')}
    </Link>
  ) : undefined

  return (
    <EmptyState
      bordered
      title={t('No recorded usage for {{period}}', {
        period: formatPeriodRange(period),
      })}
      action={action}
    />
  )
}
