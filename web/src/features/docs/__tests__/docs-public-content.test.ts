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
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { describe, expect, it } from 'vitest'

import { docsPages, parseDocsManifest } from '../lib/docs-manifest'
import { parseDocsMarkdown } from '../lib/docs-markdown'
import { docsPlaceholderValues } from '../lib/docs-placeholders'
import type { DocsManifest } from '../types'

const docsContentRoot = join(process.cwd(), 'public', 'docs-content')

describe('published documentation content', () => {
  it('passes the same parser used by the documentation route', () => {
    const manifest = parseDocsManifest(
      JSON.parse(
        readFileSync(join(docsContentRoot, 'manifest.json'), 'utf8')
      ) as DocsManifest
    )
    const placeholders = docsPlaceholderValues(
      {
        server_address: 'https://api.example.com',
        system_name: 'Example API',
      },
      'https://fallback.example.com'
    )

    for (const page of docsPages(manifest)) {
      const markdown = readFileSync(join(docsContentRoot, page.file), 'utf8')
      const document = parseDocsMarkdown(markdown, placeholders)

      expect(document.frontmatter.pageId).toBe(page.id)
      expect(document.headings[0]?.text).toBe(page.title)
    }
  })
})
