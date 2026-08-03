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
import { createHash } from 'node:crypto'
import { lstat, readFile, readdir } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

export const docsScriptDir = path.dirname(fileURLToPath(import.meta.url))
export const webRoot = path.resolve(docsScriptDir, '../..')
export const repoRoot = path.resolve(webRoot, '..')
export const contentRoot = path.join(webRoot, 'public/docs-content')
export const distContentRoot = path.join(webRoot, 'dist/docs-content')
export const manifestPath = path.join(contentRoot, 'manifest.json')
export const relayOpenAPIPath = path.join(repoRoot, 'docs/openapi/relay.json')
export const publicOperationsPath = path.join(
  repoRoot,
  'docs/openapi/public-operations.json'
)
export const generatedSearchFile = 'generated/search-index.json'

export const allowedPlaceholders = new Set([
  'SYSTEM_NAME',
  'SITE_BASE_URL',
  'OPENAI_BASE_URL',
  'ANTHROPIC_BASE_URL',
  'API_KEY_PLACEHOLDER',
  'MODEL_ID_PLACEHOLDER',
])

const slugPattern = /^[a-z0-9]+(?:-[a-z0-9]+)*(?:\/[a-z0-9]+(?:-[a-z0-9]+)*)*$/
const idPattern = /^[a-z0-9]+(?:-[a-z0-9]+)*$/
const localePattern = /^[a-z]{2}(?:-[A-Z]{2})?$/

export async function readJSON(filePath) {
  return JSON.parse(await readFile(filePath, 'utf8'))
}

export function assert(condition, message) {
  if (!condition) {
    throw new Error(message)
  }
}

export function isPlainObject(value) {
  return (
    value !== null &&
    typeof value === 'object' &&
    !Array.isArray(value) &&
    Object.getPrototypeOf(value) === Object.prototype
  )
}

function hasControlCharacter(value) {
  return [...value].some((character) => {
    const codePoint = character.codePointAt(0)
    return codePoint !== undefined && (codePoint <= 31 || codePoint === 127)
  })
}

export function assertSafeRelativePath(value, label) {
  assert(typeof value === 'string' && value.length > 0, `${label} 不能为空`)
  assert(value.length <= 240, `${label} 超过 240 字符`)
  assert(!path.isAbsolute(value), `${label} 不能是绝对路径`)
  assert(!value.includes('\\'), `${label} 不能包含反斜线`)
  assert(!value.includes('%'), `${label} 不能包含百分号编码`)
  assert(
    !value.includes('?') && !value.includes('#'),
    `${label} 不能包含查询或锚点`
  )
  assert(!hasControlCharacter(value), `${label} 不能包含控制字符`)
  assert(
    value
      .split('/')
      .every((segment) => segment && segment !== '.' && segment !== '..'),
    `${label} 包含非法路径分段`
  )
  let decoded
  try {
    decoded = decodeURIComponent(value)
  } catch {
    throw new Error(`${label} 不是有效编码`)
  }
  assert(decoded === value, `${label} 必须是规范路径`)
}

export function validateManifest(raw) {
  assert(isPlainObject(raw), 'manifest 必须是对象')
  assert(raw.schemaVersion === 1, 'manifest schemaVersion 必须为 1')
  assert(
    typeof raw.contentVersion === 'string' &&
      /^[0-9]{4}-[0-9]{2}-[0-9]{2}\.[1-9][0-9]*$/.test(raw.contentVersion),
    'manifest contentVersion 格式无效'
  )
  assert(
    typeof raw.defaultLocale === 'string' &&
      localePattern.test(raw.defaultLocale),
    'manifest defaultLocale 无效'
  )
  assert(isPlainObject(raw.locales), 'manifest locales 必须是对象')
  assert(raw.locales[raw.defaultLocale], 'manifest 缺少 defaultLocale')

  const globalPageIds = new Set()
  const localeData = {}
  let totalPages = 0

  for (const [locale, localeValue] of Object.entries(raw.locales)) {
    assert(localePattern.test(locale), `locale ${locale} 无效`)
    assert(isPlainObject(localeValue), `locale ${locale} 必须是对象`)
    assert(
      Array.isArray(localeValue.groups),
      `locale ${locale} groups 必须是数组`
    )
    assert(
      localeValue.groups.length <= 30,
      `locale ${locale} group 数量超过上限`
    )

    const groupIds = new Set()
    const pageIds = new Set()
    const slugs = new Set()
    const files = new Set()
    const pages = []

    for (const group of localeValue.groups) {
      assert(isPlainObject(group), `${locale} group 必须是对象`)
      assert(idPattern.test(group.id), `${locale} group id ${group.id} 无效`)
      assert(!groupIds.has(group.id), `${locale} group id ${group.id} 重复`)
      groupIds.add(group.id)
      assertText(group.title, `${locale}/${group.id} title`, 80)
      assert(
        Array.isArray(group.pages),
        `${locale}/${group.id} pages 必须是数组`
      )
      assert(group.pages.length > 0, `${locale}/${group.id} 不能为空组`)

      for (const page of group.pages) {
        assert(isPlainObject(page), `${locale}/${group.id} page 必须是对象`)
        assert(idPattern.test(page.id), `page id ${page.id} 无效`)
        assert(!pageIds.has(page.id), `${locale} page id ${page.id} 重复`)
        pageIds.add(page.id)
        globalPageIds.add(page.id)
        assertText(page.title, `${page.id} title`, 100)
        assertText(page.description, `${page.id} description`, 240)
        assert(
          typeof page.slug === 'string' && slugPattern.test(page.slug),
          `${page.id} slug 无效`
        )
        assertSafeRelativePath(page.slug, `${page.id} slug`)
        assert(!slugs.has(page.slug), `${locale} slug ${page.slug} 重复`)
        slugs.add(page.slug)
        assertSafeRelativePath(page.file, `${page.id} file`)
        assert(
          page.file.startsWith(`${locale}/`) && page.file.endsWith('.md'),
          `${page.id} file 必须位于 locale 目录并以 .md 结尾`
        )
        assert(!files.has(page.file), `${locale} file ${page.file} 重复`)
        files.add(page.file)
        assert(Array.isArray(page.keywords), `${page.id} keywords 必须是数组`)
        assert(page.keywords.length <= 12, `${page.id} keywords 超过上限`)
        for (const keyword of page.keywords) {
          assertText(keyword, `${page.id} keyword`, 60)
        }
        assert(Array.isArray(page.assets), `${page.id} assets 必须是数组`)
        assert(page.assets.length <= 20, `${page.id} assets 超过上限`)
        for (const asset of page.assets) {
          assertSafeRelativePath(asset, `${page.id} asset`)
          assert(
            asset.startsWith(`${locale}/assets/`),
            `${page.id} asset 必须位于 ${locale}/assets`
          )
          files.add(asset)
        }
        pages.push({
          ...page,
          groupId: group.id,
          groupTitle: group.title,
          locale,
        })
        totalPages += 1
      }
    }

    localeData[locale] = { groups: localeValue.groups, pages, files, slugs }
  }

  assert(totalPages > 0 && totalPages <= 200, 'manifest 页面总数无效')
  return { raw, localeData, totalPages, globalPageIds }
}

function assertText(value, label, maxLength) {
  assert(
    typeof value === 'string' && value.trim() === value && value.length > 0,
    `${label} 无效`
  )
  assert(value.length <= maxLength, `${label} 超过 ${maxLength} 字符`)
  assert(!hasControlCharacter(value), `${label} 包含控制字符`)
}

export function parseFrontmatter(markdown, fileLabel = 'Markdown') {
  assert(markdown.startsWith('---\n'), `${fileLabel} 缺少 frontmatter`)
  const end = markdown.indexOf('\n---\n', 4)
  assert(end >= 0, `${fileLabel} frontmatter 未闭合`)
  const lines = markdown.slice(4, end).split('\n')
  const result = { operations: [] }
  let collectingOperations = false

  for (const line of lines) {
    const operation = line.match(/^\s{2}-\s+([A-Za-z][A-Za-z0-9]+)$/)
    if (collectingOperations && operation) {
      result.operations.push(operation[1])
      continue
    }
    collectingOperations = false
    const field = line.match(/^([a-z-]+):(?:\s*(.*))?$/)
    assert(field, `${fileLabel} frontmatter 行无效: ${line}`)
    const [, key, value = ''] = field
    assert(
      ['page-id', 'kind', 'last-verified', 'operations'].includes(key),
      `${fileLabel} frontmatter 包含未知字段 ${key}`
    )
    if (key === 'operations') {
      assert(value === '[]' || value === '', `${fileLabel} operations 格式无效`)
      collectingOperations = value === ''
      continue
    }
    result[key] = value
  }

  assert(idPattern.test(result['page-id'] ?? ''), `${fileLabel} page-id 无效`)
  assert(
    result.kind === 'guide' || result.kind === 'api-reference',
    `${fileLabel} kind 无效`
  )
  assert(
    /^\d{4}-\d{2}-\d{2}$/.test(result['last-verified'] ?? ''),
    `${fileLabel} last-verified 无效`
  )
  return { frontmatter: result, body: markdown.slice(end + 5) }
}

export function stripCode(markdown) {
  return markdown.replaceAll(/```[\s\S]*?```/g, '').replaceAll(/`[^`\n]*`/g, '')
}

export function headingSlug(value, counts = new Map()) {
  const normalized =
    value
      .normalize('NFKC')
      .trim()
      .toLowerCase()
      .replaceAll(/[^\p{Letter}\p{Number}]+/gu, '-')
      .replaceAll(/^-+|-+$/g, '') || 'section'
  const count = (counts.get(normalized) ?? 0) + 1
  counts.set(normalized, count)
  return count === 1 ? normalized : `${normalized}-${count}`
}

export function extractHeadings(body) {
  const counts = new Map()
  const headings = []
  const withoutCode = body.replaceAll(/```[\s\S]*?```/g, '')
  for (const line of withoutCode.split('\n')) {
    const match = line.match(/^(#{1,3})\s+(.+?)\s*#*\s*$/)
    if (!match) continue
    const text = match[2]
      .replaceAll(/\[([^\]]+)\]\([^)]+\)/g, '$1')
      .replaceAll(/[*_`]/g, '')
      .trim()
    headings.push({
      depth: match[1].length,
      text,
      id: headingSlug(text, counts),
    })
  }
  return headings
}

export function validateContentSafety(body, page, manifestContext) {
  assert(Buffer.byteLength(body) <= 256 * 1024, `${page.file} 超过 256 KiB`)
  const withoutCode = stripCode(body)
  assert(
    !/(^|\n)\s*<(?:!doctype|\/?[a-z][^>]*)>/i.test(withoutCode),
    `${page.file} 包含原始 HTML`
  )
  assert(
    !/(^|\n)\s*(?:import|export)\s+/m.test(withoutCode),
    `${page.file} 包含 MDX`
  )
  assert(
    !/\]\(\s*(?:javascript:|data:|file:|\/\/)/i.test(body),
    `${page.file} 包含危险链接协议`
  )
  assert(!/!\[[^\]]*\]\(\s*https?:/i.test(body), `${page.file} 包含远程图片`)
  assert(!/!\[[^\]]*\]\(\s*data:/i.test(body), `${page.file} 包含内联图片`)
  assert(
    !/(?:docs\/openapi\/api\.json|private_data|header\s*override)/i.test(body),
    `${page.file} 包含禁止公开的内部字段或文件`
  )
  const placeholders = [...body.matchAll(/\{\{([A-Z0-9_]+)\}\}/g)].map(
    (match) => match[1]
  )
  for (const placeholder of placeholders) {
    assert(
      allowedPlaceholders.has(placeholder),
      `${page.file} 包含未知占位符 ${placeholder}`
    )
  }
  const unresolved = body.replaceAll(/\{\{[A-Z0-9_]+\}\}/g, '')
  assert(!/\{\{|\}\}/.test(unresolved), `${page.file} 包含格式无效的占位符`)
  const leakedKeys = body.match(/\bsk-[A-Za-z0-9_-]{12,}\b/g) ?? []
  assert(
    leakedKeys.every((value) => value === 'sk-your-key'),
    `${page.file} 疑似包含真实 API Key`
  )
  assert(
    !/Bearer\s+(?!\{\{API_KEY_PLACEHOLDER\}\}|sk-your-key)[A-Za-z0-9._-]{12,}/i.test(
      body
    ),
    `${page.file} 疑似包含真实 Bearer Token`
  )

  for (const match of withoutCode.matchAll(/(?<!!)\[[^\]]+\]\(([^)]+)\)/g)) {
    const target = match[1].trim()
    if (target.startsWith('#') || /^https?:\/\//i.test(target)) continue
    const [slug] = target.split('#', 1)
    assert(
      manifestContext.localeData[page.locale].slugs.has(slug),
      `${page.file} 链接到未登记 slug ${target}`
    )
  }
}

export async function collectFiles(root) {
  const files = []

  async function walk(directory) {
    for (const entry of await readdir(directory, { withFileTypes: true })) {
      const absolute = path.join(directory, entry.name)
      const relative = path.relative(root, absolute).split(path.sep).join('/')
      const stat = await lstat(absolute)
      assert(!stat.isSymbolicLink(), `${relative} 不能是符号链接`)
      assert(!entry.name.startsWith('.'), `${relative} 不能是隐藏文件`)
      assert(
        !/(?:~|\.tmp|\.bak|\.swp)$/.test(entry.name),
        `${relative} 是临时文件`
      )
      if (entry.isDirectory()) {
        await walk(absolute)
      } else if (entry.isFile()) {
        files.push(relative)
      } else {
        throw new Error(`${relative} 不是普通文件`)
      }
    }
  }

  await walk(root)
  return files.sort()
}

export function expectedPublicFiles(manifestContext) {
  const expected = new Set(['manifest.json', generatedSearchFile])
  for (const locale of Object.values(manifestContext.localeData)) {
    for (const file of locale.files) expected.add(file)
  }
  return [...expected].sort()
}

export function assertExactFiles(actual, expected, label) {
  const actualSet = new Set(actual)
  const expectedSet = new Set(expected)
  const unexpected = actual.filter((file) => !expectedSet.has(file))
  const missing = expected.filter((file) => !actualSet.has(file))
  assert(
    unexpected.length === 0 && missing.length === 0,
    `${label} 文件清单不一致；多余: ${unexpected.join(', ') || '无'}；缺少: ${missing.join(', ') || '无'}`
  )
}

export async function fileHash(filePath) {
  return createHash('sha256')
    .update(await readFile(filePath))
    .digest('hex')
}

export function findOpenAPIOperations(openapi) {
  const operations = new Map()
  for (const [apiPath, pathItem] of Object.entries(openapi.paths ?? {})) {
    if (!isPlainObject(pathItem)) continue
    for (const method of ['get', 'post', 'put', 'patch', 'delete']) {
      const operation = pathItem[method]
      if (!isPlainObject(operation) || !operation.operationId) continue
      assert(
        !operations.has(operation.operationId),
        `relay.json operationId ${operation.operationId} 重复`
      )
      operations.set(operation.operationId, {
        method,
        path: apiPath,
        operation,
      })
    }
  }
  return operations
}
