import { useId, type ReactNode } from 'react'

import { ReportCard } from '@/components/report-card'
import { Button } from '@/components/ui/button'

export function BillingExportSection(props: {
  title: ReactNode
  children: ReactNode
  notice?: string
  actionLabel: string
  disabled: boolean
  onSubmit: () => void
}) {
  const headingId = useId()
  return (
    <ReportCard role='region' aria-labelledby={headingId}>
      <h2 id={headingId} className='text-sm font-medium'>
        {props.title}
      </h2>
      {props.children}
      {props.notice && (
        <p className='text-muted-foreground text-xs' role='status'>
          {props.notice}
        </p>
      )}
      <Button
        size='sm'
        className='self-start'
        disabled={props.disabled}
        onClick={props.onSubmit}
      >
        {props.actionLabel}
      </Button>
    </ReportCard>
  )
}
