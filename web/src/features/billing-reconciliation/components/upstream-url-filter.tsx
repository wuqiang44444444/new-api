import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Combobox,
  ComboboxInput,
  ComboboxContent,
  ComboboxList,
  ComboboxItem,
  useComboboxAnchor,
} from '@/components/ui/combobox'
import { useDebounce } from '@/hooks/use-debounce'

import { useUpstreamPage } from '../upstream-page-query'
import {
  upstreamUrlGroupLabel,
  upstreamUrlGroupSubtitle,
} from '../upstream-reconciliation-utils'
import { BillingPagination } from './billing-pagination'

export function UpstreamURLFilter(props: {
  period: { start_timestamp: number; end_timestamp: number }
  value: { value: string; label: string } | null
  onChange: (value: { value: string; label: string } | null) => void
}) {
  const { t } = useTranslation()
  const anchor = useComboboxAnchor()
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const debouncedSearch = useDebounce(search, 250)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const query = useUpstreamPage(
    {
      ...props.period,
      level: 'options',
      search: debouncedSearch,
      page,
      page_size: pageSize,
    },
    open
  )
  const items = [
    { value: 'all', label: t('All upstream URLs') },
    ...(query.data?.result.url_groups ?? []).map((group) => ({
      value: group.url_key,
      label: group.custom_name
        ? `${upstreamUrlGroupLabel(group, t)} · ${upstreamUrlGroupSubtitle(group, t)}`
        : upstreamUrlGroupLabel(group, t),
    })),
  ]
  return (
    <Combobox
      items={items}
      value={props.value}
      open={open}
      inputValue={
        open ? search : (props.value?.label ?? t('All upstream URLs'))
      }
      filter={null}
      isItemEqualToValue={(a, b) => a.value === b.value}
      onOpenChange={(value) => {
        setOpen(value)
        setSearch('')
        setPage(1)
      }}
      onInputValueChange={(value, details) => {
        if (details.reason === 'input-change') {
          setSearch(value)
          setPage(1)
        }
      }}
      onValueChange={(value) => {
        if (value) props.onChange(value.value === 'all' ? null : value)
      }}
    >
      <div ref={anchor}>
        <ComboboxInput
          id='upstream-url-filter'
          aria-label={t('Upstream base URL')}
          triggerAriaLabel={t('Upstream base URL')}
        />
      </div>
      <ComboboxContent anchor={anchor}>
        {query.isError ? (
          <Button variant='ghost' onClick={() => void query.refetch()}>
            {t('Retry')}
          </Button>
        ) : null}
        <ComboboxList>
          {(item: { value: string; label: string }) => (
            <ComboboxItem key={item.value} value={item}>
              {item.label}
            </ComboboxItem>
          )}
        </ComboboxList>
        <BillingPagination
          label={t('Upstream URL pages')}
          page={page}
          pageSize={pageSize}
          total={query.data?.result.total ?? 0}
          disabled={query.isFetching}
          onChange={(p, size) => {
            setPage(p)
            setPageSize(size)
          }}
        />
      </ComboboxContent>
    </Combobox>
  )
}
