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
import { marked, type Token, type Tokens } from 'marked'

import type { DocsHeading, DocsPlaceholderValues } from '../types'
import { parseDocsFrontmatter, type DocsFrontmatter } from './docs-frontmatter'
import { docsHeadingId } from './docs-headings'
import { replaceDocsPlaceholders } from './docs-placeholders'

export type DocsMarkdownBlock = {
  headingId?: string
  token: Token
}

export type DocsMarkdownDocument = {
  blocks: DocsMarkdownBlock[]
  frontmatter: DocsFrontmatter
  headings: DocsHeading[]
}

const allowedTokens = new Set([
  'space',
  'code',
  'heading',
  'table',
  'hr',
  'blockquote',
  'list',
  'list_item',
  'paragraph',
  'text',
  'escape',
  'strong',
  'em',
  'codespan',
  'br',
  'del',
  'link',
])

function childTokens(value: unknown): Token[] {
  if (!value || typeof value !== 'object') return []
  const object = value as Record<string, unknown>
  const children: Token[] = []
  if (Array.isArray(object.tokens)) children.push(...(object.tokens as Token[]))
  if (Array.isArray(object.items)) children.push(...(object.items as Token[]))
  if (Array.isArray(object.header)) {
    for (const cell of object.header as Array<{ tokens?: Token[] }>) {
      if (Array.isArray(cell.tokens)) children.push(...cell.tokens)
    }
  }
  if (Array.isArray(object.rows)) {
    for (const row of object.rows as Array<Array<{ tokens?: Token[] }>>) {
      for (const cell of row) {
        if (Array.isArray(cell.tokens)) children.push(...cell.tokens)
      }
    }
  }
  return children
}

function validateMarkdownTokens(tokens: Token[]): void {
  for (const token of tokens) {
    if (!allowedTokens.has(token.type)) {
      throw new Error(`Unsupported documentation Markdown token: ${token.type}`)
    }
    if (token.type === 'link') {
      const href = (token as Tokens.Link).href.trim()
      const hasUnsafeCharacter = [...href].some((character) => {
        const code = character.codePointAt(0) ?? 0
        return code <= 31 || code === 127 || character === '\\'
      })
      if (
        /^(?:javascript:|data:|file:|\/\/)/i.test(href) ||
        hasUnsafeCharacter
      ) {
        throw new Error('Documentation contains an unsafe link.')
      }
    }
    validateMarkdownTokens(childTokens(token))
  }
}

export function docsTokenText(
  token: Token,
  placeholders: DocsPlaceholderValues
): string {
  const children = childTokens(token)
  if (children.length > 0) {
    return children.map((child) => docsTokenText(child, placeholders)).join('')
  }
  if ('text' in token && typeof token.text === 'string') {
    return replaceDocsPlaceholders(token.text, placeholders)
  }
  return ''
}

export function parseDocsMarkdown(
  markdown: string,
  placeholders: DocsPlaceholderValues
): DocsMarkdownDocument {
  const { body, frontmatter } = parseDocsFrontmatter(markdown)
  replaceDocsPlaceholders(body, placeholders)
  const tokens = marked.lexer(body, { breaks: false, gfm: true }) as Token[]
  validateMarkdownTokens(tokens)

  const counts = new Map<string, number>()
  const headings: DocsHeading[] = []
  const blocks = tokens.map((token): DocsMarkdownBlock => {
    if (token.type !== 'heading') return { token }
    const heading = token as Tokens.Heading
    if (heading.depth < 1 || heading.depth > 3) {
      throw new Error('Documentation headings are limited to h1 through h3.')
    }
    const text = docsTokenText(token, placeholders)
    const headingId = docsHeadingId(text, counts)
    headings.push({
      depth: heading.depth as 1 | 2 | 3,
      id: headingId,
      text,
    })
    return { headingId, token }
  })

  if (headings.filter((heading) => heading.depth === 1).length !== 1) {
    throw new Error('Documentation must contain exactly one h1.')
  }
  return { blocks, frontmatter, headings }
}
