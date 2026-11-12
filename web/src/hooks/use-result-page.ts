import { useState } from 'react'

export function useResultPage(total: number, scope?: string | number) {
  const [selection, setSelection] = useState({ page: 1, pageSize: 10, scope })
  const { pageSize } = selection
  const page =
    selection.scope === scope
      ? Math.min(selection.page, Math.max(1, Math.ceil(total / pageSize)))
      : 1
  const offset = (page - 1) * pageSize

  return {
    page,
    pageSize,
    total,
    offset,
    onChange: (nextPage: number, nextSize: number) =>
      setSelection({ page: nextPage, pageSize: nextSize, scope }),
  }
}
