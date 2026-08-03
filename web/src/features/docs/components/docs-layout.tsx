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
import {
  ArrowLeft01Icon,
  ArrowRight01Icon,
  BookOpen01Icon,
  Menu01Icon,
  Task01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { Link } from '@tanstack/react-router'
import { type ReactNode, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ThemeSwitch } from '@/components/theme-switch'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet'

import type { DocsHeading, DocsManifest, DocsPage } from '../types'
import { DocsNavigation } from './docs-navigation'
import { DocsSearch } from './docs-search'
import { DocsTableOfContents } from './docs-toc'

export function DocsLayout({
  activePage,
  children,
  headings,
  locale,
  manifest,
  nextPage,
  previousPage,
  systemName,
}: {
  activePage: DocsPage
  children: ReactNode
  headings: DocsHeading[]
  locale: string
  manifest: DocsManifest
  nextPage: DocsPage | null
  previousPage: DocsPage | null
  systemName: string
}) {
  const { t } = useTranslation()
  const [navigationOpen, setNavigationOpen] = useState(false)
  const [tocOpen, setTocOpen] = useState(false)

  return (
    <div className='bg-background min-h-svh'>
      <header className='bg-background/95 supports-backdrop-filter:bg-background/80 sticky top-0 z-40 border-b backdrop-blur'>
        <div className='mx-auto flex h-14 max-w-[96rem] items-center gap-2 px-3 sm:px-5'>
          <Link
            to='/'
            className='mr-auto flex min-w-0 items-center gap-2 font-semibold'
          >
            <span className='bg-primary text-primary-foreground flex size-8 shrink-0 items-center justify-center rounded-lg'>
              <HugeiconsIcon icon={BookOpen01Icon} strokeWidth={2} />
            </span>
            <span className='truncate'>{systemName}</span>
            <span className='text-muted-foreground hidden font-normal sm:inline'>
              {t('Docs')}
            </span>
          </Link>
          <DocsSearch locale={locale} manifest={manifest} />
          <ThemeSwitch />
        </div>
        <div className='flex gap-2 border-t px-3 py-2 xl:hidden'>
          <Sheet open={navigationOpen} onOpenChange={setNavigationOpen}>
            <SheetTrigger
              render={
                <Button
                  variant='outline'
                  className='flex-1 lg:hidden'
                  aria-expanded={navigationOpen}
                />
              }
            >
              <HugeiconsIcon icon={Menu01Icon} strokeWidth={2} />
              {t('Browse docs')}
            </SheetTrigger>
            <SheetContent side='left' className='w-[88%] sm:max-w-sm'>
              <SheetHeader>
                <SheetTitle>{t('Documentation')}</SheetTitle>
                <SheetDescription>
                  {t('Browse all published API documentation.')}
                </SheetDescription>
              </SheetHeader>
              <div className='overflow-y-auto px-3 pb-6'>
                <DocsNavigation
                  activeSlug={activePage.slug}
                  locale={locale}
                  manifest={manifest}
                  onNavigate={() => setNavigationOpen(false)}
                />
              </div>
            </SheetContent>
          </Sheet>
          <Sheet open={tocOpen} onOpenChange={setTocOpen}>
            <SheetTrigger
              render={
                <Button
                  variant='outline'
                  className='flex-1 xl:hidden'
                  aria-expanded={tocOpen}
                />
              }
            >
              <HugeiconsIcon icon={Task01Icon} strokeWidth={2} />
              {t('On this page')}
            </SheetTrigger>
            <SheetContent side='right' className='w-[88%] sm:max-w-sm'>
              <SheetHeader>
                <SheetTitle>{t('On this page')}</SheetTitle>
                <SheetDescription>
                  {t('Jump to a section on this page.')}
                </SheetDescription>
              </SheetHeader>
              <div className='overflow-y-auto px-4 pb-6'>
                <DocsTableOfContents
                  headings={headings}
                  onNavigate={() => setTocOpen(false)}
                />
              </div>
            </SheetContent>
          </Sheet>
        </div>
      </header>

      <div className='mx-auto grid max-w-[96rem] lg:grid-cols-[15rem_minmax(0,1fr)] xl:grid-cols-[15rem_minmax(0,52rem)_14rem]'>
        <aside className='hidden border-r lg:block'>
          <div className='sticky top-14 h-[calc(100svh-3.5rem)] overflow-y-auto px-4 py-7'>
            <DocsNavigation
              activeSlug={activePage.slug}
              locale={locale}
              manifest={manifest}
            />
          </div>
        </aside>

        <main className='min-w-0 px-4 py-10 sm:px-8 lg:px-10'>
          {children}
          <nav
            aria-label={t('Documentation pagination')}
            className='mt-12 grid gap-3 border-t pt-6 sm:grid-cols-2'
          >
            {previousPage ? (
              <Link
                to='/docs/$'
                params={{ _splat: previousPage.slug }}
                className='hover:bg-muted rounded-xl border p-4 transition-colors'
              >
                <span className='text-muted-foreground flex items-center gap-1 text-xs'>
                  <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
                  {t('Previous')}
                </span>
                <span className='mt-1 block font-medium'>
                  {previousPage.title}
                </span>
              </Link>
            ) : (
              <span />
            )}
            {nextPage && (
              <Link
                to='/docs/$'
                params={{ _splat: nextPage.slug }}
                className='hover:bg-muted rounded-xl border p-4 text-right transition-colors'
              >
                <span className='text-muted-foreground flex items-center justify-end gap-1 text-xs'>
                  {t('Next')}
                  <HugeiconsIcon icon={ArrowRight01Icon} strokeWidth={2} />
                </span>
                <span className='mt-1 block font-medium'>{nextPage.title}</span>
              </Link>
            )}
          </nav>
        </main>

        <aside className='hidden xl:block'>
          <div className='sticky top-14 max-h-[calc(100svh-3.5rem)] overflow-y-auto px-5 py-10'>
            <DocsTableOfContents headings={headings} />
          </div>
        </aside>
      </div>
    </div>
  )
}
