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
import { z } from 'zod'

import type { DocsManifest, DocsSearchIndex } from '../types'
import { docsPages } from './docs-manifest'

const searchText = z.string().min(1).max(240)
const headingSchema = z
  .object({
    depth: z.union([z.literal(2), z.literal(3)]),
    id: z
      .string()
      .min(1)
      .max(160)
      .regex(/^[\p{Letter}\p{Number}]+(?:-[\p{Letter}\p{Number}]+)*$/u),
    text: searchText,
  })
  .strict()
const entrySchema = z
  .object({
    pageId: z.string().min(1).max(80),
    slug: z.string().min(1).max(200),
    title: z.string().min(1).max(100),
    description: searchText,
    keywords: z.array(z.string().min(1).max(60)).max(12),
    headings: z.array(headingSchema).max(80),
  })
  .strict()
const searchIndexSchema = z
  .object({
    schemaVersion: z.literal(1),
    contentVersion: z.string(),
    locales: z.record(z.string(), z.array(entrySchema).max(200)),
  })
  .strict()

export function parseDocsSearchIndex(
  value: unknown,
  manifest: DocsManifest
): DocsSearchIndex {
  const index = searchIndexSchema.parse(value)
  if (index.contentVersion !== manifest.contentVersion) {
    throw new Error('The documentation search index version is invalid.')
  }

  for (const [locale, localeValue] of Object.entries(index.locales)) {
    const manifestPages = docsPages(manifest, locale)
    const expected = new Map(manifestPages.map((page) => [page.id, page]))
    if (!manifest.locales[locale] || localeValue.length !== expected.size) {
      throw new Error('The documentation search index locale is invalid.')
    }
    for (const entry of localeValue) {
      const page = expected.get(entry.pageId)
      if (
        !page ||
        entry.slug !== page.slug ||
        entry.title !== page.title ||
        entry.description !== page.description ||
        entry.keywords.length !== page.keywords.length ||
        entry.keywords.some(
          (keyword, index) => keyword !== page.keywords[index]
        )
      ) {
        throw new Error(
          'The documentation search index does not match the manifest.'
        )
      }
      expected.delete(entry.pageId)
    }
  }

  if (
    Object.keys(index.locales).length !== Object.keys(manifest.locales).length
  ) {
    throw new Error('The documentation search index is missing a locale.')
  }
  return index
}
