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
import type { DocsManifest, DocsPage } from '../types'
import { docsPages } from './docs-manifest'

export type ResolvedMarkdownLink =
  | { kind: 'external'; href: string }
  | { kind: 'hash'; href: string }
  | { kind: 'internal'; hash: string; page: DocsPage }

export function resolveMarkdownLink(
  href: string,
  manifest: DocsManifest,
  locale: string
): ResolvedMarkdownLink | null {
  if (href.startsWith('#') && /^#[\p{Letter}\p{Number}-]+$/u.test(href)) {
    return { kind: 'hash', href }
  }
  try {
    const url = new URL(href)
    if (
      (url.protocol === 'http:' || url.protocol === 'https:') &&
      !url.username &&
      !url.password
    ) {
      return { kind: 'external', href: url.href }
    }
  } catch {
    // Relative links are resolved as logical manifest slugs below.
  }

  const hashIndex = href.indexOf('#')
  const slug = hashIndex >= 0 ? href.slice(0, hashIndex) : href
  const hash = hashIndex >= 0 ? href.slice(hashIndex + 1) : ''
  if (
    !/^[a-z0-9]+(?:-[a-z0-9]+)*(?:\/[a-z0-9]+(?:-[a-z0-9]+)*)*$/.test(slug) ||
    (hash && !/^[\p{Letter}\p{Number}-]+$/u.test(hash))
  ) {
    return null
  }
  const page = docsPages(manifest, locale).find((item) => item.slug === slug)
  return page ? { kind: 'internal', hash, page } : null
}
