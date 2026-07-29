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
import type { DocsManifest, DocsPage, DocsSearchIndex } from '../types'
import { parseDocsManifest } from './docs-manifest'
import { parseDocsSearchIndex } from './docs-search-index'

async function checkedJSON(response: Response): Promise<unknown> {
  if (!response.ok) {
    throw new Error(`Documentation request failed with ${response.status}.`)
  }
  return response.json()
}

export async function loadDocsManifest(
  signal?: AbortSignal
): Promise<DocsManifest> {
  const response = await fetch('/docs-content/manifest.json', {
    cache: 'no-store',
    credentials: 'same-origin',
    signal,
  })
  return parseDocsManifest(await checkedJSON(response))
}

export async function loadDocsMarkdown(
  page: DocsPage,
  contentVersion: string,
  signal?: AbortSignal
): Promise<string> {
  const response = await fetch(
    `/docs-content/${page.file}?v=${encodeURIComponent(contentVersion)}`,
    {
      cache: 'force-cache',
      credentials: 'same-origin',
      signal,
    }
  )
  if (!response.ok) {
    throw new Error(`Documentation content failed with ${response.status}.`)
  }
  return response.text()
}

export async function loadDocsSearchIndex(
  manifest: DocsManifest,
  signal?: AbortSignal
): Promise<DocsSearchIndex> {
  const response = await fetch(
    `/docs-content/generated/search-index.json?v=${encodeURIComponent(manifest.contentVersion)}`,
    {
      cache: 'force-cache',
      credentials: 'same-origin',
      signal,
    }
  )
  return parseDocsSearchIndex(await checkedJSON(response), manifest)
}
