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

import type { DocsManifest, DocsPage } from '../types'

const safeId = z
  .string()
  .regex(/^[a-z0-9]+(?:-[a-z0-9]+)*$/)
  .max(80)
const safeSlug = z
  .string()
  .regex(/^[a-z0-9]+(?:-[a-z0-9]+)*(?:\/[a-z0-9]+(?:-[a-z0-9]+)*)*$/)
  .max(200)
const safeFile = z
  .string()
  .regex(/^[a-z]{2}(?:-[A-Z]{2})?\/[a-z0-9][a-z0-9/-]*\.md$/)
  .max(240)
const safeAsset = z
  .string()
  .max(240)
  .refine(
    (value) =>
      !value.includes('\\') &&
      !value.includes('%') &&
      !value.includes('?') &&
      !value.includes('#') &&
      value
        .split('/')
        .every((segment) => segment && segment !== '.' && segment !== '..')
  )

const pageSchema = z
  .object({
    id: safeId,
    title: z.string().min(1).max(100),
    slug: safeSlug,
    file: safeFile,
    description: z.string().min(1).max(240),
    keywords: z.array(z.string().min(1).max(60)).max(12),
    assets: z.array(safeAsset).max(20),
  })
  .strict()

const manifestSchema = z
  .object({
    schemaVersion: z.literal(1),
    contentVersion: z.string().regex(/^\d{4}-\d{2}-\d{2}\.[1-9]\d*$/),
    defaultLocale: z.string().regex(/^[a-z]{2}(?:-[A-Z]{2})?$/),
    locales: z
      .record(
        z.string(),
        z
          .object({
            groups: z
              .array(
                z
                  .object({
                    id: safeId,
                    title: z.string().min(1).max(80),
                    pages: z.array(pageSchema).min(1),
                  })
                  .strict()
              )
              .max(30),
          })
          .strict()
      )
      .refine((locales) => Object.keys(locales).length > 0),
  })
  .strict()

export function parseDocsManifest(value: unknown): DocsManifest {
  const manifest = manifestSchema.parse(value)
  if (!manifest.locales[manifest.defaultLocale]) {
    throw new Error('The default documentation locale is missing.')
  }

  for (const [locale, localeValue] of Object.entries(manifest.locales)) {
    const ids = new Set<string>()
    const slugs = new Set<string>()
    const files = new Set<string>()
    for (const group of localeValue.groups) {
      for (const page of group.pages) {
        if (
          ids.has(page.id) ||
          slugs.has(page.slug) ||
          files.has(page.file) ||
          !page.file.startsWith(`${locale}/`) ||
          page.assets.some((asset) => !asset.startsWith(`${locale}/assets/`)) ||
          decodeURIComponent(page.slug) !== page.slug
        ) {
          throw new Error(
            'The documentation manifest contains duplicate or unsafe paths.'
          )
        }
        ids.add(page.id)
        slugs.add(page.slug)
        files.add(page.file)
      }
    }
  }

  return manifest
}

export function docsPages(
  manifest: DocsManifest,
  locale = manifest.defaultLocale
): DocsPage[] {
  const selected =
    manifest.locales[locale] ?? manifest.locales[manifest.defaultLocale]
  return selected.groups.flatMap((group) => group.pages)
}

export function resolveDocsPage(
  manifest: DocsManifest,
  rawSlug: string | undefined,
  locale = manifest.defaultLocale
): DocsPage | null {
  const slug = rawSlug || 'overview'
  if (
    !/^[a-z0-9]+(?:-[a-z0-9]+)*(?:\/[a-z0-9]+(?:-[a-z0-9]+)*)*$/.test(slug) ||
    decodeURIComponent(slug) !== slug
  ) {
    return null
  }
  return docsPages(manifest, locale).find((page) => page.slug === slug) ?? null
}
