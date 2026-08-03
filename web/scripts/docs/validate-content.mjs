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
import { readFile } from 'node:fs/promises'
import path from 'node:path'

import {
  assert,
  contentRoot,
  extractHeadings,
  parseFrontmatter,
  validateContentSafety,
} from './common.mjs'
import { validateDocsManifest } from './validate-manifest.mjs'

export async function validateDocsContent(manifestContext) {
  const context = manifestContext ?? (await validateDocsManifest())
  const documents = []

  for (const locale of Object.values(context.localeData)) {
    for (const page of locale.pages) {
      const markdown = await readFile(path.join(contentRoot, page.file), 'utf8')
      const { frontmatter, body } = parseFrontmatter(markdown, page.file)
      assert(
        frontmatter['page-id'] === page.id,
        `${page.file} page-id 与 manifest 不一致`
      )
      assert(
        frontmatter.kind === 'api-reference' ||
          frontmatter.operations.length === 0,
        `${page.file} 普通指南不能绑定 operation`
      )
      assert(
        frontmatter.kind !== 'api-reference' ||
          frontmatter.operations.length > 0,
        `${page.file} API Reference 必须绑定 operation`
      )
      validateContentSafety(body, page, context)
      const headings = extractHeadings(body)
      const h1 = headings.filter((heading) => heading.depth === 1)
      assert(h1.length === 1, `${page.file} 必须且只能包含一个 h1`)
      assert(
        h1[0].text === page.title,
        `${page.file} h1 与 manifest title 不一致`
      )
      documents.push({ page, frontmatter, body, headings, markdown })
    }
  }

  return { context, documents }
}

if (process.argv[1] === new URL(import.meta.url).pathname) {
  await validateDocsContent()
  console.log('Docs content validation passed.')
}
