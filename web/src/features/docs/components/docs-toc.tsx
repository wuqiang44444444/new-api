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
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import type { DocsHeading } from '../types'

export function DocsTableOfContents({
  headings,
  onNavigate,
}: {
  headings: DocsHeading[]
  onNavigate?: () => void
}) {
  const { t } = useTranslation()
  const sections = useMemo(
    () => headings.filter((heading) => heading.depth !== 1),
    [headings]
  )
  const [activeId, setActiveId] = useState(sections[0]?.id ?? '')

  useEffect(() => {
    setActiveId(sections[0]?.id ?? '')
    if (sections.length === 0) return

    const elements = sections
      .map((section) =>
        document.querySelector<HTMLElement>(`#${CSS.escape(section.id)}`)
      )
      .filter((element): element is HTMLElement => element !== null)
    const observer = new IntersectionObserver(
      (entries) => {
        const visible = entries
          .filter((entry) => entry.isIntersecting)
          .sort(
            (left, right) =>
              left.boundingClientRect.top - right.boundingClientRect.top
          )
        if (visible[0]?.target.id) setActiveId(visible[0].target.id)
      },
      { rootMargin: '-80px 0px -70% 0px' }
    )
    elements.forEach((element) => observer.observe(element))
    return () => observer.disconnect()
  }, [sections])

  if (sections.length === 0) return null

  return (
    <nav aria-label={t('On this page')}>
      <h2 className='mb-3 text-sm font-semibold'>{t('On this page')}</h2>
      <ul className='border-l'>
        {sections.map((heading) => (
          <li key={heading.id}>
            <a
              href={`#${heading.id}`}
              onClick={onNavigate}
              className={cn(
                '-ml-px block border-l px-3 py-1.5 text-sm transition-colors',
                heading.depth === 3 && 'pl-6',
                activeId === heading.id
                  ? 'border-primary text-primary'
                  : 'text-muted-foreground hover:text-foreground border-transparent'
              )}
            >
              {heading.text}
            </a>
          </li>
        ))}
      </ul>
    </nav>
  )
}
