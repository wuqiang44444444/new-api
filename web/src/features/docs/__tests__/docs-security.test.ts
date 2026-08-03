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

import { resolveDocsLink } from '@/lib/docs-link'
import {
  resolveOpenAIBaseUrl,
  resolvePublicSiteBaseUrl,
} from '@/lib/public-api-base-url'

import { parseDocsMarkdown } from '../lib/docs-markdown'
import { docsPlaceholderValues } from '../lib/docs-placeholders'

const placeholders = docsPlaceholderValues(
  {
    server_address: 'https://api.example.com/v1/',
    system_name: 'Example API',
  },
  'https://fallback.example.com'
)

function markdown(body: string) {
  return `---
page-id: quickstart
kind: guide
last-verified: 2026-07-29
operations: []
---

${body}`
}

describe('documentation security boundaries', () => {
  it('resolves docs links without allowing unsafe protocols or credentials', () => {
    expect(resolveDocsLink('https://docs.example.com/path')).toEqual({
      external: true,
      href: 'https://docs.example.com/path',
    })
    expect(resolveDocsLink('/docs/quickstart')).toEqual({
      external: false,
      href: '/docs/quickstart',
    })
    expect(resolveDocsLink('javascript:alert(1)')).toEqual({
      external: false,
      href: '/docs',
    })
    expect(resolveDocsLink('//example.com')).toEqual({
      external: false,
      href: '/docs',
    })
    expect(resolveDocsLink('https://user:secret@example.com')).toEqual({
      external: false,
      href: '/docs',
    })
  })

  it('normalizes public base URLs and removes only a terminal v1', () => {
    const status = { server_address: 'https://api.example.com/prefix/v1/' }
    expect(resolvePublicSiteBaseUrl(status)).toBe(
      'https://api.example.com/prefix'
    )
    expect(resolveOpenAIBaseUrl(status)).toBe(
      'https://api.example.com/prefix/v1'
    )
  })

  it('builds stable duplicate heading ids and preserves placeholders as text', () => {
    const document = parseDocsMarkdown(
      markdown(
        '# {{SYSTEM_NAME}}\n\n## 重试\n\n## 重试\n\n```bash\ncurl {{OPENAI_BASE_URL}}\n```'
      ),
      placeholders
    )

    expect(document.headings.map((heading) => heading.id)).toEqual([
      'example-api',
      '重试',
      '重试-2',
    ])
  })

  it('accepts an allowlisted multiline operations field', () => {
    const document = parseDocsMarkdown(
      `---
page-id: video-tasks
kind: api-reference
last-verified: 2026-07-29
operations:
  - createVideoTask
  - getVideoTask
---

# 视频任务`,
      placeholders
    )

    expect(document.frontmatter.operations).toEqual([
      'createVideoTask',
      'getVideoTask',
    ])
  })

  it.each([
    '# 标题\n\n<script>alert(1)</script>',
    '# 标题\n\n[危险](javascript:alert(1))',
    '# 标题\n\n![远程图](https://example.com/image.png)',
    '# 标题\n\n{{UNKNOWN_VALUE}}',
  ])('rejects unsafe Markdown: %s', (body) => {
    expect(() => parseDocsMarkdown(markdown(body), placeholders)).toThrow()
  })
})
