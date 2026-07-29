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
import path from 'node:path'

import {
  assert,
  assertExactFiles,
  collectFiles,
  distContentRoot,
  expectedPublicFiles,
  fileHash,
  generatedSearchFile,
  manifestPath,
  readJSON,
  validateManifest,
  webRoot,
} from './common.mjs'

export async function validateDocsDist() {
  const sourceContext = validateManifest(await readJSON(manifestPath))
  const expected = expectedPublicFiles(sourceContext)
  const actual = await collectFiles(distContentRoot)
  assertExactFiles(actual, expected, 'docs-content 构建产物')

  for (const file of expected) {
    const source = path.join(webRoot, 'public/docs-content', file)
    const built = path.join(distContentRoot, file)
    assert(
      (await fileHash(source)) === (await fileHash(built)),
      `构建产物 ${file} 与源文件不一致`
    )
  }

  const search = await readJSON(path.join(distContentRoot, generatedSearchFile))
  assert(search.schemaVersion === 1, '构建搜索索引 schemaVersion 无效')
  assert(
    search.contentVersion === sourceContext.raw.contentVersion,
    '构建搜索索引 contentVersion 与 manifest 不一致'
  )
  const serialized = JSON.stringify(search)
  assert(
    !/docs\/openapi\/api\.json|private_data/i.test(serialized),
    '搜索索引包含内部内容'
  )
}

if (process.argv[1] === new URL(import.meta.url).pathname) {
  await validateDocsDist()
  console.log('Docs dist validation passed.')
}
