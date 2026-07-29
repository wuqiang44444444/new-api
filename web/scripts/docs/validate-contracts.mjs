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
import {
  assert,
  findOpenAPIOperations,
  publicOperationsPath,
  readJSON,
  relayOpenAPIPath,
} from './common.mjs'
import { validateDocsContent } from './validate-content.mjs'

export async function validateDocsContracts(contentResult) {
  const content = contentResult ?? (await validateDocsContent())
  const [openapi, allowlist] = await Promise.all([
    readJSON(relayOpenAPIPath),
    readJSON(publicOperationsPath),
  ])
  assert(openapi.openapi === '3.0.1', 'relay.json OpenAPI 版本无效')
  assert(
    allowlist.schemaVersion === 1,
    'public operations schemaVersion 必须为 1'
  )
  assert(Array.isArray(allowlist.operations), 'public operations 必须是数组')

  const openapiOperations = findOpenAPIOperations(openapi)
  const approved = new Set()
  for (const item of allowlist.operations) {
    assert(
      item &&
        typeof item.operationId === 'string' &&
        item.status === 'published' &&
        typeof item.family === 'string',
      'public operations 条目无效'
    )
    assert(
      !approved.has(item.operationId),
      `public operation ${item.operationId} 重复`
    )
    approved.add(item.operationId)
    const found = openapiOperations.get(item.operationId)
    assert(found, `public operation ${item.operationId} 不在 relay.json`)
    assert(
      !found.operation.deprecated,
      `public operation ${item.operationId} 已废弃`
    )
    assert(
      !found.path.startsWith('x-retired-'),
      `public operation ${item.operationId} 指向退休路径`
    )
    assert(
      found.operation.security?.some((entry) => entry.BearerAuth),
      `public operation ${item.operationId} 缺少 BearerAuth`
    )
    assert(
      found.operation.responses &&
        Object.keys(found.operation.responses).length > 0,
      `public operation ${item.operationId} 缺少响应 schema`
    )
  }

  const referenced = new Set()
  for (const document of content.documents) {
    for (const operationId of document.frontmatter.operations) {
      assert(
        approved.has(operationId),
        `${document.page.file} 引用了未公开 operation ${operationId}`
      )
      referenced.add(operationId)
    }
  }
  for (const operationId of approved) {
    assert(
      referenced.has(operationId),
      `public operation ${operationId} 没有对应 API Reference`
    )
  }

  return { ...content, openapi, allowlist, openapiOperations, approved }
}

if (process.argv[1] === new URL(import.meta.url).pathname) {
  await validateDocsContracts()
  console.log('Docs contract validation passed.')
}
