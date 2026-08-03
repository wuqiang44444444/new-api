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
import { mkdir, writeFile } from 'node:fs/promises'
import path from 'node:path'

import { contentRoot, generatedSearchFile } from './common.mjs'
import { validateDocsContracts } from './validate-contracts.mjs'

function formatSearchIndex(searchIndex) {
  return JSON.stringify(searchIndex, null, 2).replaceAll(
    /"keywords": \[\n((?:\s+"(?:[^"\\]|\\.)*"(?:,)?\n)+)\s+\]/g,
    (_match, keywordLines) =>
      `"keywords": [${keywordLines
        .trim()
        .split('\n')
        .map((line) => line.trim())
        .join(' ')}]`
  )
}

export async function buildDocsSearchIndex(contractResult) {
  const contract = contractResult ?? (await validateDocsContracts())
  const locales = {}
  for (const locale of Object.keys(contract.context.localeData)) {
    locales[locale] = contract.documents
      .filter((document) => document.page.locale === locale)
      .map((document) => ({
        pageId: document.page.id,
        slug: document.page.slug,
        title: document.page.title,
        description: document.page.description,
        keywords: document.page.keywords,
        headings: document.headings
          .filter((heading) => heading.depth === 2 || heading.depth === 3)
          .map(({ depth, id, text }) => ({ depth, id, text })),
      }))
  }

  const searchIndex = {
    schemaVersion: 1,
    contentVersion: contract.context.raw.contentVersion,
    locales,
  }
  const outputPath = path.join(contentRoot, generatedSearchFile)
  await mkdir(path.dirname(outputPath), { recursive: true })
  await writeFile(outputPath, `${formatSearchIndex(searchIndex)}\n`, 'utf8')
  return { contract, searchIndex }
}

if (process.argv[1] === new URL(import.meta.url).pathname) {
  await buildDocsSearchIndex()
  console.log('Docs search index generated.')
}
