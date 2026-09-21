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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { FileStack, RefreshCw, Search } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatTimestamp } from '@/lib/format'

import { getContractTemplates } from '../template-api'
import type { ContractTemplateListItem } from '../template-types'
import { ContractTemplateDrawer } from './contract-template-drawer'

const TEMPLATE_PAGE_SIZE = 20

function TemplateStatusBadge(props: { enabled: boolean }) {
  const { t } = useTranslation()
  return (
    <Badge variant={props.enabled ? 'secondary' : 'outline'}>
      {props.enabled ? t('Enabled') : t('Disabled')}
    </Badge>
  )
}

export function ContractTemplateManager() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [keyword, setKeyword] = useState('')
  const [queryKeyword, setQueryKeyword] = useState('')
  const [enabledFilter, setEnabledFilter] = useState<'all' | 'true' | 'false'>(
    'all'
  )
  const [page, setPage] = useState(1)
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [editTemplateId, setEditTemplateId] = useState<number | null>(null)

  const query = useQuery({
    queryKey: [
      'contract-templates',
      page,
      queryKeyword,
      enabledFilter,
    ],
    queryFn: async () => {
      const response = await getContractTemplates({
        p: page,
        page_size: TEMPLATE_PAGE_SIZE,
        keyword: queryKeyword,
        enabled: enabledFilter === 'all' ? '' : enabledFilter === 'true',
      })
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Failed to load contract templates'))
      }
      return response.data
    },
  })

  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ['contract-templates'] })
  }

  const items = query.data?.items ?? []
  const total = query.data?.total ?? 0
  const pageCount = Math.max(1, Math.ceil(total / TEMPLATE_PAGE_SIZE))

  const openEdit = (templateId: number) => {
    setEditTemplateId(templateId)
    setDrawerOpen(true)
  }

  const closeDrawer = (nextOpen: boolean) => {
    if (!nextOpen) {
      setEditTemplateId(null)
      setDrawerOpen(false)
    }
  }

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Breadcrumb>
        <div className='flex items-center gap-1 text-sm text-muted-foreground'>
          <Link
            to='/admin/customer-contracts'
            className='hover:text-foreground'
          >
            {t('Customer model contracts')}
          </Link>
          <span>/</span>
          <span className='text-foreground'>{t('Contract templates')}</span>
        </div>
      </SectionPageLayout.Breadcrumb>
      <SectionPageLayout.Title>{t('Contract templates')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button
          type='button'
          variant='outline'
          size='sm'
          disabled={query.isFetching}
          onClick={() => void query.refetch()}
        >
          <RefreshCw aria-hidden='true' data-icon='inline-start' />
          {t('Refresh')}
        </Button>
        <Button
          type='button'
          size='sm'
          onClick={() => {
            setEditTemplateId(null)
            setDrawerOpen(true)
          }}
        >
          <FileStack aria-hidden='true' data-icon='inline-start' />
          {t('New contract template')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='flex h-full min-h-0 flex-col gap-4 overflow-hidden'>
          <div className='flex shrink-0 flex-col gap-2 sm:flex-row sm:items-center'>
            <div className='relative w-full sm:max-w-xs'>
              <Search className='text-muted-foreground absolute top-1/2 left-2.5 h-4 w-4 -translate-y-1/2' />
              <Input
                value={keyword}
                onChange={(event) => setKeyword(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter') {
                    setPage(1)
                    setQueryKeyword(keyword.trim())
                  }
                }}
                placeholder={t('Search template names...')}
                className='pl-8'
              />
            </div>
            <Select
              items={[
                { value: 'all', label: t('All templates') },
                { value: 'true', label: t('Enabled') },
                { value: 'false', label: t('Disabled') },
              ]}
              value={enabledFilter}
              onValueChange={(value) => {
                setEnabledFilter((value as 'all' | 'true' | 'false') || 'all')
                setPage(1)
              }}
            >
              <SelectTrigger className='w-40'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem value='all'>{t('All templates')}</SelectItem>
                  <SelectItem value='true'>{t('Enabled')}</SelectItem>
                  <SelectItem value='false'>{t('Disabled')}</SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>

          {query.isError ? (
            <div
              role='alert'
              className='flex-1 rounded-xl border p-8 text-center'
            >
              <p className='font-medium'>
                {t('Failed to load contract templates')}
              </p>
              <p className='text-muted-foreground mt-1 text-sm'>
                {query.error.message}
              </p>
              <Button
                variant='outline'
                size='sm'
                className='mt-3'
                onClick={() => void query.refetch()}
              >
                {t('Retry')}
              </Button>
              <p className='text-muted-foreground text-sm'>
                {t('Templates only prefill new customer contracts.')}
              </p>
            </div>
          ) : (
            <div className='min-h-0 flex-1 overflow-auto rounded-xl border'>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('Template name')}</TableHead>
                    <TableHead>{t('Status')}</TableHead>
                    <TableHead>{t('Models')}</TableHead>
                    <TableHead>{t('Rules')}</TableHead>
                    <TableHead>{t('Stale rules')}</TableHead>
                    <TableHead>{t('Updated by')}</TableHead>
                    <TableHead>{t('Updated at')}</TableHead>
                    <TableHead />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map((item) => (
                    <TemplateRow
                      key={item.id}
                      item={item}
                      onEdit={() => openEdit(item.id)}
                    />
                  ))}
                </TableBody>
              </Table>
              {items.length === 0 && !query.isPending && (
                <div className='text-muted-foreground p-8 text-center text-sm'>
                  {t('No contract templates')}
                </div>
              )}
              {items.length === 0 && query.isPending && (
                <div className='text-muted-foreground p-8 text-center text-sm'>
                  {t('Loading...')}
                </div>
              )}
            </div>
          )}

          <div className='flex shrink-0 items-center justify-end gap-2'>
            <Button
              type='button'
              variant='outline'
              size='sm'
              disabled={page <= 1}
              onClick={() => setPage(page - 1)}
            >
              {t('Previous')}
            </Button>
            <span className='text-muted-foreground text-sm'>
              {page} / {pageCount}
            </span>
            <Button
              type='button'
              variant='outline'
              size='sm'
              disabled={page >= pageCount}
              onClick={() => setPage(page + 1)}
            >
              {t('Next')}
            </Button>
          </div>
        </div>

        {drawerOpen && (
          <ContractTemplateDrawer
            open
            onOpenChange={closeDrawer}
            templateId={editTemplateId}
            onSaved={refresh}
          />
        )}
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

function TemplateRow(props: { item: ContractTemplateListItem; onEdit: () => void }) {
  const { t } = useTranslation()
  return (
    <TableRow>
      <TableCell className='max-w-56 truncate font-medium'>
        {props.item.name}
      </TableCell>
      <TableCell>
        <TemplateStatusBadge enabled={props.item.enabled} />
      </TableCell>
      <TableCell>{props.item.model_count}</TableCell>
      <TableCell>{props.item.rule_count}</TableCell>
      <TableCell>
        {props.item.stale_rule_count > 0 ? (
          <Badge variant='destructive'>
            {t('{{count}} stale', { count: props.item.stale_rule_count })}
          </Badge>
          ) : (
          <span className='text-muted-foreground text-sm'>0</span>
        )}
      </TableCell>
      <TableCell className='text-muted-foreground text-sm'>
        {props.item.updater_name || `#${props.item.updater_id}`}
      </TableCell>
      <TableCell className='text-muted-foreground text-sm'>
        {formatTimestamp(props.item.updated_at)}
      </TableCell>
      <TableCell>
        <Button type='button' variant='outline' size='sm' onClick={props.onEdit}>
          {t('Edit')}
        </Button>
      </TableCell>
    </TableRow>
  )
}
