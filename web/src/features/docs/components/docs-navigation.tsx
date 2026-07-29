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
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import type { DocsManifest } from '../types'

export function DocsNavigation({
  activeSlug,
  locale,
  manifest,
  onNavigate,
}: {
  activeSlug: string
  locale: string
  manifest: DocsManifest
  onNavigate?: () => void
}) {
  const { t } = useTranslation()
  const selected =
    manifest.locales[locale] ?? manifest.locales[manifest.defaultLocale]

  return (
    <nav aria-label={t('Documentation')}>
      <div className='space-y-6'>
        {selected.groups.map((group) => (
          <section key={group.id}>
            <h2 className='text-muted-foreground mb-2 px-2 text-xs font-semibold tracking-wide uppercase'>
              {group.title}
            </h2>
            <ul className='space-y-0.5'>
              {group.pages.map((page) => (
                <li key={page.id}>
                  <Link
                    to='/docs/$'
                    params={{ _splat: page.slug }}
                    onClick={onNavigate}
                    aria-current={page.slug === activeSlug ? 'page' : undefined}
                    className={cn(
                      'block rounded-lg px-2 py-1.5 text-sm transition-colors',
                      page.slug === activeSlug
                        ? 'bg-primary/10 text-primary font-medium'
                        : 'text-muted-foreground hover:bg-muted hover:text-foreground'
                    )}
                  >
                    {page.title}
                  </Link>
                </li>
              ))}
            </ul>
          </section>
        ))}
      </div>
    </nav>
  )
}
