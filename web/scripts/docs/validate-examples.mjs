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
import Ajv from 'ajv'
import addFormats from 'ajv-formats'

import { assert, findOpenAPIOperations } from './common.mjs'

// Validate against the actual OpenAPI schemas, without maintaining a second
// request/response contract in the documentation tooling.
export function validateOpenAPIExamples(openapi, approved) {
  const ajv = new Ajv({ strict: false, allErrors: true })
  addFormats(ajv)
  ajv.addFormat('binary', true)
  ajv.addFormat('int64', { type: 'number', validate: Number.isInteger })
  ajv.addSchema(openapi, 'relay')
  const pending = []
  for (const [id, entry] of findOpenAPIOperations(openapi)) {
    if (!approved.has(id)) continue
    const apiPath = entry.path.replaceAll('~', '~0').replaceAll('/', '~1')
    pending.push([entry.operation, `relay#/paths/${apiPath}/${entry.method}`])
  }
  const visited = new Set()
  let count = 0
  while (pending.length) {
    const [node, pointer] = pending.pop()
    if (!node || typeof node !== 'object' || visited.has(pointer)) continue
    visited.add(pointer)
    if (node.$ref) {
      assert(node.$ref.startsWith('#/'), `OpenAPI 仅允许本地引用: ${pointer}`)
      const segments = node.$ref
        .slice(2)
        .split('/')
        .map((key) => key.replaceAll('~1', '/').replaceAll('~0', '~'))
      const referenced = segments.reduce((value, key) => value?.[key], openapi)
      assert(referenced, `OpenAPI 引用不存在: ${pointer} ${node.$ref}`)
      pending.push([referenced, `relay${node.$ref}`])
    }
    const propertyMap = pointer.endsWith('/properties')
    // Schema examples and media-type / parameter examples use different owners.
    const schemaPointer = node.schema ? `${pointer}/schema` : pointer
    const examples = []
    if (!propertyMap && Object.hasOwn(node, 'example')) {
      examples.push(node.example)
    }
    if (node.schema && node.examples) {
      for (const example of Object.values(node.examples)) {
        if (Object.hasOwn(example, 'value')) examples.push(example.value)
      }
    }
    for (const example of examples) {
      const validate = ajv.getSchema(schemaPointer)
      assert(validate, `OpenAPI 示例没有 schema: ${schemaPointer}`)
      assert(
        validate(example),
        `OpenAPI 示例不符合合同: ${schemaPointer} ${ajv.errorsText(validate.errors)}`
      )
      count += 1
    }
    for (const [key, value] of Object.entries(node)) {
      if (
        !propertyMap &&
        ['example', 'examples', 'default', 'enum'].includes(key)
      ) {
        continue
      }
      const escaped = key.replaceAll('~', '~0').replaceAll('/', '~1')
      const childPointer = `${pointer}/${escaped}`
      pending.push([value, childPointer])
    }
  }
  return count
}

export function validateMarkdownJSONExamples(documents) {
  let count = 0
  for (const document of documents) {
    const blocks = document.body.matchAll(/^```(json|bash)\n([\s\S]*?)^```/gm)
    for (const [, language, body] of blocks) {
      const examples =
        language === 'json'
          ? [body]
          : [
              ...body.matchAll(
                /(?:-d|--data|--data-raw|--data-binary)\s+'([^']*)'/g
              ),
            ]
              .map((match) => match[1])
              .filter(
                (value) =>
                  value.trimStart().startsWith('{') ||
                  value.trimStart().startsWith('[')
              )
      for (const example of examples) {
        try {
          JSON.parse(example)
        } catch {
          throw new Error(`Markdown JSON 示例无效: ${document.page.file}`)
        }
        count += 1
      }
    }
  }
  return count
}
