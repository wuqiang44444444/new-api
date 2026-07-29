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
export type DocsFrontmatter = {
  pageId: string
  kind: 'guide' | 'api-reference'
  lastVerified: string
  operations: string[]
}

export function parseDocsFrontmatter(markdown: string): {
  body: string
  frontmatter: DocsFrontmatter
} {
  if (!markdown.startsWith('---\n')) {
    throw new Error('Documentation frontmatter is missing.')
  }
  const end = markdown.indexOf('\n---\n', 4)
  if (end < 0) {
    throw new Error('Documentation frontmatter is not closed.')
  }

  const fields = new Map<string, string>()
  const operations: string[] = []
  let collectingOperations = false
  for (const line of markdown.slice(4, end).split('\n')) {
    const operation = line.match(/^\s{2}-\s+([A-Za-z][A-Za-z0-9]+)$/)
    if (collectingOperations && operation) {
      operations.push(operation[1])
      continue
    }
    collectingOperations = false
    const match = line.match(/^([a-z-]+):(?:\s*(.*))?$/)
    if (
      !match ||
      !['page-id', 'kind', 'last-verified', 'operations'].includes(match[1])
    ) {
      throw new Error('Documentation frontmatter contains an invalid field.')
    }
    if (match[1] === 'operations') {
      const operationsValue = match[2] ?? ''
      if (operationsValue !== '' && operationsValue !== '[]') {
        throw new Error('Documentation operations are invalid.')
      }
      collectingOperations = operationsValue === ''
    } else {
      fields.set(match[1], match[2] ?? '')
    }
  }

  const kind = fields.get('kind')
  const pageId = fields.get('page-id') ?? ''
  const lastVerified = fields.get('last-verified') ?? ''
  if (
    !/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(pageId) ||
    (kind !== 'guide' && kind !== 'api-reference') ||
    !/^\d{4}-\d{2}-\d{2}$/.test(lastVerified)
  ) {
    throw new Error('Documentation frontmatter is invalid.')
  }

  return {
    body: markdown.slice(end + 5),
    frontmatter: { pageId, kind, lastVerified, operations },
  }
}
