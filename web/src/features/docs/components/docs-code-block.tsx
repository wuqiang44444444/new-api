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
import { Copy01Icon, Tick02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { type CSSProperties, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { BundledLanguage, SpecialLanguage } from 'shiki'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

type HighlightedToken = {
  id: string
  content: string
  dark?: string
  light?: string
}

type HighlightedLine = {
  id: string
  tokens: HighlightedToken[]
}

const highlightCache = new Map<string, Promise<HighlightedLine[]>>()

function highlightCode(
  code: string,
  language: string
): Promise<HighlightedLine[]> {
  const cacheKey = `${language}\0${code}`
  const cached = highlightCache.get(cacheKey)
  if (cached) return cached

  const result = import('shiki').then(async ({ codeToTokensWithThemes }) => {
    const lines = await codeToTokensWithThemes(code, {
      lang: (language || 'text') as BundledLanguage | SpecialLanguage,
      themes: {
        dark: 'github-dark',
        light: 'github-light',
      },
    })
    let offset = 0
    return lines.map((line) => {
      const lineId = `line-${offset}`
      const tokens = line.map((token) => {
        const highlightedToken = {
          id: `token-${offset}`,
          content: token.content,
          dark: token.variants.dark?.color,
          light: token.variants.light?.color,
        }
        offset += token.content.length
        return highlightedToken
      })
      offset += 1
      return { id: lineId, tokens }
    })
  })
  highlightCache.set(cacheKey, result)
  return result
}

export function DocsCodeBlock({
  code,
  language,
}: {
  code: string
  language: string
}) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)
  const [lines, setLines] = useState<HighlightedLine[] | null>(null)

  useEffect(() => {
    let active = true
    void highlightCode(code, language)
      .then((value) => {
        if (active) setLines(value)
      })
      .catch(() => {
        if (active) setLines(null)
      })
    return () => {
      active = false
    }
  }, [code, language])

  async function copyCode() {
    try {
      await navigator.clipboard.writeText(code)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1600)
    } catch {
      setCopied(false)
    }
  }

  return (
    <div className='group/code bg-muted/40 my-6 overflow-hidden rounded-xl border'>
      <div className='text-muted-foreground flex min-h-9 items-center justify-between border-b px-3 text-xs'>
        <span>{language || t('Plain text')}</span>
        <Button
          type='button'
          variant='ghost'
          size='sm'
          onClick={copyCode}
          aria-label={copied ? t('Copied') : t('Copy code')}
        >
          <HugeiconsIcon
            icon={copied ? Tick02Icon : Copy01Icon}
            strokeWidth={2}
          />
          {copied ? t('Copied') : t('Copy')}
        </Button>
      </div>
      <pre className='overflow-x-auto p-4 text-[13px] leading-6'>
        <code className={cn(lines && 'shiki')}>
          {lines
            ? lines.map((line) => (
                <span key={line.id} className='block min-h-6'>
                  {line.tokens.map((token) => {
                    const style = {
                      '--shiki-dark': token.dark,
                      '--shiki-light': token.light,
                    } as CSSProperties
                    return (
                      <span key={token.id} style={style}>
                        {token.content}
                      </span>
                    )
                  })}
                </span>
              ))
            : code}
        </code>
      </pre>
    </div>
  )
}
