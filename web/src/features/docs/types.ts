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
export type DocsPage = {
  id: string
  title: string
  slug: string
  file: string
  description: string
  keywords: string[]
  assets: string[]
}

export type DocsGroup = {
  id: string
  title: string
  pages: DocsPage[]
}

export type DocsLocale = {
  groups: DocsGroup[]
}

export type DocsManifest = {
  schemaVersion: 1
  contentVersion: string
  defaultLocale: string
  locales: Record<string, DocsLocale>
}

export type DocsHeading = {
  depth: 1 | 2 | 3
  id: string
  text: string
}

export type DocsSearchEntry = {
  pageId: string
  slug: string
  title: string
  description: string
  keywords: string[]
  headings: Array<{
    depth: 2 | 3
    id: string
    text: string
  }>
}

export type DocsSearchIndex = {
  schemaVersion: 1
  contentVersion: string
  locales: Record<string, DocsSearchEntry[]>
}

export type DocsPlaceholderValues = Record<
  | 'SYSTEM_NAME'
  | 'SITE_BASE_URL'
  | 'OPENAI_BASE_URL'
  | 'ANTHROPIC_BASE_URL'
  | 'API_KEY_PLACEHOLDER'
  | 'MODEL_ID_PLACEHOLDER',
  string
>
