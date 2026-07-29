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

import {
  docsPages,
  parseDocsManifest,
  resolveDocsPage,
} from '../lib/docs-manifest'
import { parseDocsSearchIndex } from '../lib/docs-search-index'

function manifestFixture() {
  return {
    schemaVersion: 1,
    contentVersion: '2026-07-29.1',
    defaultLocale: 'zh',
    locales: {
      zh: {
        groups: [
          {
            id: 'start',
            title: '开始',
            pages: [
              {
                id: 'quickstart',
                title: '快速开始',
                slug: 'quickstart',
                file: 'zh/quickstart.md',
                description: '开始调用',
                keywords: ['API'],
                assets: [],
              },
            ],
          },
        ],
      },
    },
  }
}

describe('documentation manifest', () => {
  it('parses a bounded manifest and resolves only registered slugs', () => {
    const manifest = parseDocsManifest(manifestFixture())

    expect(docsPages(manifest)).toHaveLength(1)
    expect(resolveDocsPage(manifest, 'quickstart')?.file).toBe(
      'zh/quickstart.md'
    )
    expect(resolveDocsPage(manifest, 'unknown')).toBeNull()
  })

  it.each([
    '../quickstart',
    'api/%2e%2e/private',
    'api\\private',
    '//example.com',
    'Quickstart',
  ])('rejects non-canonical route input %s', (slug) => {
    const manifest = parseDocsManifest(manifestFixture())
    expect(resolveDocsPage(manifest, slug)).toBeNull()
  })

  it('rejects duplicate and escaped manifest paths', () => {
    const fixture = manifestFixture()
    fixture.locales.zh.groups[0].pages.push({
      ...fixture.locales.zh.groups[0].pages[0],
      id: 'second',
      file: 'zh/second.md',
    })

    expect(() => parseDocsManifest(fixture)).toThrow()
  })

  it('accepts only search entries that exactly match the manifest allowlist', () => {
    const manifest = parseDocsManifest(manifestFixture())
    const searchIndex = {
      schemaVersion: 1,
      contentVersion: manifest.contentVersion,
      locales: {
        zh: [
          {
            pageId: 'quickstart',
            slug: 'quickstart',
            title: '快速开始',
            description: '开始调用',
            keywords: ['API'],
            headings: [{ depth: 2, id: 'request', text: '发送请求' }],
          },
        ],
      },
    } as const

    expect(parseDocsSearchIndex(searchIndex, manifest)).toEqual(searchIndex)
    expect(() =>
      parseDocsSearchIndex(
        {
          ...searchIndex,
          locales: {
            zh: [{ ...searchIndex.locales.zh[0], slug: '../private' }],
          },
        },
        manifest
      )
    ).toThrow()
  })
})
