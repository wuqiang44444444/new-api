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
import { useQuery } from '@tanstack/react-query'
import { Link, useRouterState } from '@tanstack/react-router'
import { useEffect, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useStatus } from '@/hooks/use-status'
import { resolvePublicSystemName } from '@/lib/public-api-base-url'

import { DocsContent } from './components/docs-content'
import { DocsLayout } from './components/docs-layout'
import { loadDocsManifest, loadDocsMarkdown } from './lib/docs-loader'
import { docsPages, resolveDocsPage } from './lib/docs-manifest'
import { parseDocsMarkdown } from './lib/docs-markdown'
import { docsPlaceholderValues } from './lib/docs-placeholders'

function DocsLoading() {
  return (
    <div className='mx-auto max-w-3xl space-y-5 px-5 py-16'>
      <Skeleton className='h-10 w-2/3' />
      <Skeleton className='h-5 w-full' />
      <Skeleton className='h-5 w-5/6' />
      <Skeleton className='mt-10 h-8 w-1/3' />
      <Skeleton className='h-32 w-full' />
    </div>
  )
}

function DocsUnavailable({
  description,
  onRetry,
  title,
}: {
  description: string
  onRetry?: () => void
  title: string
}) {
  const { t } = useTranslation()
  return (
    <div className='mx-auto flex min-h-svh max-w-xl items-center px-5'>
      <Alert variant='destructive'>
        <AlertTitle>{title}</AlertTitle>
        <AlertDescription>{description}</AlertDescription>
        <div className='mt-4 flex gap-2'>
          {onRetry && (
            <Button type='button' variant='outline' onClick={onRetry}>
              {t('Retry')}
            </Button>
          )}
          <Button render={<Link to='/' />} variant='outline'>
            {t('Back to home')}
          </Button>
        </div>
      </Alert>
    </div>
  )
}

export function DocsPage({ slug }: { slug?: string }) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const location = useRouterState({ select: (state) => state.location })
  const manifestQuery = useQuery({
    queryKey: ['docs-manifest'],
    queryFn: ({ signal }) => loadDocsManifest(signal),
    staleTime: Number.POSITIVE_INFINITY,
  })
  const manifest = manifestQuery.data
  const locale = manifest?.locales.zh ? 'zh' : manifest?.defaultLocale
  const page = manifest ? resolveDocsPage(manifest, slug, locale) : null
  const contentQuery = useQuery({
    queryKey: ['docs-content', manifest?.contentVersion, page?.id],
    queryFn: ({ signal }) => {
      if (!page || !manifest) {
        throw new Error('Documentation content was requested without a page.')
      }
      return loadDocsMarkdown(page, manifest.contentVersion, signal)
    },
    enabled: Boolean(manifest && page),
    staleTime: Number.POSITIVE_INFINITY,
  })
  const placeholders = useMemo(() => docsPlaceholderValues(status), [status])
  const parsed = useMemo(() => {
    if (!contentQuery.data || !page) return null
    try {
      const document = parseDocsMarkdown(contentQuery.data, placeholders)
      if (
        document.frontmatter.pageId !== page.id ||
        document.headings[0]?.text !== page.title
      ) {
        throw new Error('Documentation metadata does not match the manifest.')
      }
      return { document, error: null }
    } catch (error) {
      return {
        document: null,
        error:
          error instanceof Error
            ? error
            : new Error('Documentation rendering failed.'),
      }
    }
  }, [contentQuery.data, page, placeholders])

  useEffect(() => {
    if (!page) return
    document.title = `${page.title} · ${resolvePublicSystemName(status)}`
  }, [page, status])

  useEffect(() => {
    if (!parsed?.document || !page) return
    const frame = window.requestAnimationFrame(() => {
      const hash = location.hash.replace(/^#/, '')
      let headingId = ''
      try {
        headingId = decodeURIComponent(hash)
      } catch {
        // Ignore malformed user-provided fragments.
      }
      const target = headingId
        ? document.querySelector<HTMLElement>(`#${CSS.escape(headingId)}`)
        : document.querySelector<HTMLElement>('.docs-content h1')
      target?.focus({ preventScroll: Boolean(headingId) })
      if (headingId) target?.scrollIntoView({ block: 'start' })
    })
    return () => window.cancelAnimationFrame(frame)
  }, [location.hash, page, parsed])

  if (manifestQuery.isLoading) return <DocsLoading />
  if (manifestQuery.isError || !manifest) {
    return (
      <DocsUnavailable
        title={t('Documentation is unavailable')}
        description={t(
          'The documentation manifest could not be loaded or did not pass validation.'
        )}
        onRetry={() => void manifestQuery.refetch()}
      />
    )
  }
  if (!page || !locale) {
    return (
      <DocsUnavailable
        title={t('Documentation page not found')}
        description={t(
          'This page is not part of the published documentation manifest.'
        )}
      />
    )
  }
  if (contentQuery.isLoading) return <DocsLoading />
  if (contentQuery.isError || !contentQuery.data) {
    return (
      <DocsUnavailable
        title={t('Documentation content is unavailable')}
        description={t('The selected documentation page could not be loaded.')}
        onRetry={() => void contentQuery.refetch()}
      />
    )
  }
  if (!parsed?.document || parsed.error) {
    return (
      <DocsUnavailable
        title={t('Documentation content was rejected')}
        description={t(
          'The selected page did not pass the safe rendering checks.'
        )}
      />
    )
  }

  const pages = docsPages(manifest, locale)
  const pageIndex = pages.findIndex((item) => item.id === page.id)
  return (
    <DocsLayout
      activePage={page}
      headings={parsed.document.headings}
      locale={locale}
      manifest={manifest}
      previousPage={pages[pageIndex - 1] ?? null}
      nextPage={pages[pageIndex + 1] ?? null}
      systemName={resolvePublicSystemName(status)}
    >
      <DocsContent
        document={parsed.document}
        locale={locale}
        manifest={manifest}
        placeholders={placeholders}
      />
    </DocsLayout>
  )
}
