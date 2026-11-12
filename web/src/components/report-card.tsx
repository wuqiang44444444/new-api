import type { ComponentProps, ReactNode } from 'react'

import { Card } from '@/components/ui/card'
import { cn } from '@/lib/utils'

export function ReportCard(props: ComponentProps<typeof Card>) {
  return (
    <Card
      data-card-hover='false'
      {...props}
      className={cn(
        'min-w-0 shrink-0 gap-2 rounded-lg border px-3 py-2 ring-0',
        props.className
      )}
    />
  )
}

type ReportMetricProps = {
  label: ReactNode
  value: ReactNode
  icon?: ReactNode
  caption?: ReactNode
  emphasized?: boolean
  mutedValue?: boolean
  stacked?: boolean
}

export function ReportMetric(props: ReportMetricProps) {
  return (
    <div className='max-w-full min-w-0'>
      <div
        className={cn(
          'flex gap-x-2 gap-y-0.5',
          props.stacked ? 'flex-col items-start' : 'flex-wrap items-baseline'
        )}
      >
        <div className='text-muted-foreground flex items-center gap-1.5 text-xs font-medium'>
          {props.icon && (
            <span
              aria-hidden='true'
              className={cn('shrink-0', props.emphasized && 'text-primary')}
            >
              {props.icon}
            </span>
          )}
          {props.label}
        </div>
        <p
          className={cn(
            'max-w-full overflow-x-auto text-base/5 font-semibold tracking-tight whitespace-nowrap tabular-nums',
            props.mutedValue && 'text-muted-foreground'
          )}
        >
          {props.value}
        </p>
      </div>
      {props.caption && (
        <p className='text-muted-foreground text-xs'>{props.caption}</p>
      )}
    </div>
  )
}

export function ReportMetricCard({
  children,
  className,
  ...metric
}: ReportMetricProps & {
  children?: ReactNode
  className?: string
}) {
  return (
    <ReportCard
      className={cn(
        'flex-row flex-wrap items-center gap-x-2 gap-y-1 py-1.5',
        metric.emphasized && 'border-primary/25 bg-primary/5',
        className
      )}
    >
      <ReportMetric {...metric} />
      {children && (
        <div className='@container min-w-[min(100%,8rem)] flex-1 border-s ps-2'>
          {children}
        </div>
      )}
    </ReportCard>
  )
}
