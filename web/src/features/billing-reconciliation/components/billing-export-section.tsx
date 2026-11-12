import { useId, type ReactNode } from 'react'

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
    <section
      aria-labelledby={headingId}
      className='space-y-3 rounded-lg border p-3'
    >
      <h2 id={headingId} className='text-sm font-medium'>
        {props.title}
      </h2>
      {props.children}
      {props.notice && (
        <p className='text-muted-foreground text-xs' role='status'>
          {props.notice}
        </p>
      )}
      <Button size='sm' disabled={props.disabled} onClick={props.onSubmit}>
        {props.actionLabel}
      </Button>
    </section>
  )
}
