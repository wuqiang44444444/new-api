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
import { describe, expect, it } from 'vitest'

import { searchDocsEntries } from '../lib/docs-search'
import type { DocsSearchEntry } from '../types'

const entries: DocsSearchEntry[] = [
  {
    pageId: 'images',
    slug: 'api-reference/images',
    title: '图片生成',
    description: '创建图片并查询任务',
    keywords: ['prompt'],
    headings: [{ depth: 2, id: 'tasks', text: '异步任务' }],
  },
  {
    pageId: 'auth',
    slug: 'authentication',
    title: '鉴权',
    description: '安全使用 Bearer Key',
    keywords: ['API Key'],
    headings: [],
  },
]

describe('documentation search', () => {
  it('searches public metadata and ranks title matches first', () => {
    expect(
      searchDocsEntries(entries, '图片').map((entry) => entry.pageId)
    ).toEqual(['images'])
    expect(searchDocsEntries(entries, '异步任务')[0]?.pageId).toBe('images')
    expect(searchDocsEntries(entries, 'Bearer')[0]?.pageId).toBe('auth')
  })

  it('returns no result for body text that is not indexed', () => {
    expect(searchDocsEntries(entries, '未索引正文')).toEqual([])
  })
})
