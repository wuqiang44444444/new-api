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
import { Search01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'

import { loadDocsSearchIndex } from '../lib/docs-loader'
import { searchDocsEntries } from '../lib/docs-search'
import type { DocsManifest } from '../types'

export function DocsSearch({
  locale,
  manifest,
}: {
  locale: string
  manifest: DocsManifest
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const search = useQuery({
    queryKey: ['docs-search', manifest.contentVersion],
    queryFn: ({ signal }) => loadDocsSearchIndex(manifest, signal),
    enabled: open,
    staleTime: Number.POSITIVE_INFINITY,
  })

  useEffect(() => {
    const handleShortcut = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault()
        setOpen(true)
      }
    }
    window.addEventListener('keydown', handleShortcut)
    return () => window.removeEventListener('keydown', handleShortcut)
  }, [])

  const entries = useMemo(
    () => search.data?.locales[locale] ?? [],
    [locale, search.data?.locales]
  )
  const results = useMemo(
    () => searchDocsEntries(entries, query),
    [entries, query]
  )

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        setOpen(nextOpen)
        if (!nextOpen) setQuery('')
      }}
    >
      <DialogTrigger
        render={
          <Button
            variant='outline'
            className='text-muted-foreground justify-start sm:w-56'
          />
        }
      >
        <HugeiconsIcon icon={Search01Icon} strokeWidth={2} />
        <span className='hidden sm:inline'>{t('Search docs')}</span>
        <kbd className='bg-muted ml-auto hidden rounded px-1.5 py-0.5 text-[10px] sm:inline'>
          ⌘K
        </kbd>
      </DialogTrigger>
      <DialogContent className='top-[18%] max-h-[70vh] translate-y-0 overflow-hidden p-0 sm:max-w-xl'>
        <DialogHeader className='sr-only'>
          <DialogTitle>{t('Search docs')}</DialogTitle>
          <DialogDescription>
            {t('Search page titles, descriptions, keywords, and headings.')}
          </DialogDescription>
        </DialogHeader>
        <div className='border-b p-3'>
          <div className='relative'>
            <HugeiconsIcon
              icon={Search01Icon}
              strokeWidth={2}
              className='text-muted-foreground pointer-events-none absolute top-2 left-2.5 size-4'
            />
            <Input
              autoFocus
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder={t('Search docs')}
              aria-label={t('Search docs')}
              className='pl-8'
            />
          </div>
        </div>
        <div className='min-h-32 overflow-y-auto p-2'>
          {search.isLoading && (
            <p className='text-muted-foreground p-4 text-sm'>
              {t('Loading search index...')}
            </p>
          )}
          {search.isError && (
            <p className='text-destructive p-4 text-sm'>
              {t('Search is temporarily unavailable.')}
            </p>
          )}
          {search.isSuccess && results.length === 0 && (
            <p className='text-muted-foreground p-4 text-sm'>
              {t('No documentation results found.')}
            </p>
          )}
          {results.map((entry) => (
            <Link
              key={entry.pageId}
              to='/docs/$'
              params={{ _splat: entry.slug }}
              onClick={() => setOpen(false)}
              className='hover:bg-muted focus-visible:bg-muted block rounded-lg p-3 outline-none'
            >
              <span className='block text-sm font-medium'>{entry.title}</span>
              <span className='text-muted-foreground mt-1 line-clamp-2 block text-xs'>
                {entry.description}
              </span>
            </Link>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  )
}
