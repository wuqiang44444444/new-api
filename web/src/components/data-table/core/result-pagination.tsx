import { getCoreRowModel, useReactTable } from '@tanstack/react-table'

import { FieldSet } from '@/components/ui/field'

import { DataTablePagination, type PendingPageTotal } from './pagination'

// Reuse the shared table paging controls for paged report results.
export function ResultPagination(props: {
  page: number
  pageSize: number
  total: number
  onChange: (page: number, pageSize: number) => void
  label: string
  disabled?: boolean
  allowPageJump?: boolean
  pageSizes?: readonly number[]
  pendingTotal?: PendingPageTotal
  onLastPage?: () => void
}) {
  const pagination = { pageIndex: props.page - 1, pageSize: props.pageSize }
  const table = useReactTable({
    data: [],
    columns: [],
    getCoreRowModel: getCoreRowModel(),
    manualPagination: true,
    rowCount: props.total,
    pageCount: props.pendingTotal ? -1 : undefined,
    state: { pagination },
    onPaginationChange: (update) => {
      if (props.disabled) return
      const next = typeof update === 'function' ? update(pagination) : update
      props.onChange(
        next.pageSize === props.pageSize ? next.pageIndex + 1 : 1,
        next.pageSize
      )
    },
  })
  return (
    <FieldSet
      className='min-w-0'
      aria-label={props.label}
      disabled={props.disabled}
      aria-busy={props.disabled}
    >
      <DataTablePagination
        table={table}
        pageSizes={props.pageSizes}
        showPageCount
        allowPageJump={props.allowPageJump}
        pendingTotal={props.pendingTotal}
        onLastPage={props.onLastPage}
      />
    </FieldSet>
  )
}
