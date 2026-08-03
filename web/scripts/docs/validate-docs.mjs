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
import { buildDocsSearchIndex } from './build-search-index.mjs'
import {
  assertExactFiles,
  collectFiles,
  contentRoot,
  expectedPublicFiles,
} from './common.mjs'

export async function validateDocs() {
  const { contract } = await buildDocsSearchIndex()
  const actual = await collectFiles(contentRoot)
  const expected = expectedPublicFiles(contract.context)
  assertExactFiles(actual, expected, 'docs-content 源目录')
  return contract
}

if (process.argv[1] === new URL(import.meta.url).pathname) {
  const contract = await validateDocs()
  console.log(
    `Docs validation passed: ${contract.context.totalPages} pages, ${contract.approved.size} operations.`
  )
}
