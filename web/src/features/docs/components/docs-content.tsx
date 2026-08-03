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
import { Link } from '@tanstack/react-router'
import type { Token, Tokens } from 'marked'
import { Fragment, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { resolveMarkdownLink } from '../lib/docs-links'
import type { DocsMarkdownDocument } from '../lib/docs-markdown'
import { replaceDocsPlaceholders } from '../lib/docs-placeholders'
import type { DocsManifest, DocsPlaceholderValues } from '../types'
import { DocsCodeBlock } from './docs-code-block'

type RendererContext = {
  locale: string
  manifest: DocsManifest
  placeholders: DocsPlaceholderValues
}

function withStableKeys<T>(
  items: T[],
  getIdentity: (item: T) => string
): { item: T; key: string }[] {
  let offset = 0
  return items.map((item) => {
    const identity = getIdentity(item)
    const key = `${offset}:${identity}`
    offset += identity.length + 1
    return { item, key }
  })
}

function renderTokens(tokens: Token[] | undefined, context: RendererContext) {
  return withStableKeys(tokens ?? [], (token) => token.raw).map(
    ({ item: token, key }) => (
      <Fragment key={key}>{renderToken(token, context)}</Fragment>
    )
  )
}

function renderLink(token: Tokens.Link, context: RendererContext): ReactNode {
  const resolved = resolveMarkdownLink(
    token.href.trim(),
    context.manifest,
    context.locale
  )
  const children = renderTokens(token.tokens, context)
  if (!resolved) return <span>{children}</span>
  if (resolved.kind === 'external') {
    return (
      <a href={resolved.href} target='_blank' rel='noopener noreferrer'>
        {children}
      </a>
    )
  }
  if (resolved.kind === 'hash') {
    return <a href={resolved.href}>{children}</a>
  }
  return (
    <Link
      to='/docs/$'
      params={{ _splat: resolved.page.slug }}
      hash={resolved.hash || undefined}
    >
      {children}
    </Link>
  )
}

function renderToken(token: Token, context: RendererContext): ReactNode {
  switch (token.type) {
    case 'space':
      return null
    case 'escape':
    case 'text': {
      const textToken = token as Tokens.Text
      return textToken.tokens?.length
        ? renderTokens(textToken.tokens, context)
        : replaceDocsPlaceholders(textToken.text, context.placeholders)
    }
    case 'strong':
      return (
        <strong>
          {renderTokens((token as Tokens.Strong).tokens, context)}
        </strong>
      )
    case 'em':
      return <em>{renderTokens((token as Tokens.Em).tokens, context)}</em>
    case 'del':
      return <del>{renderTokens((token as Tokens.Del).tokens, context)}</del>
    case 'codespan':
      return (
        <code className='bg-muted rounded px-1.5 py-0.5 font-mono text-[0.9em]'>
          {replaceDocsPlaceholders(
            (token as Tokens.Codespan).text,
            context.placeholders
          )}
        </code>
      )
    case 'br':
      return <br />
    case 'link':
      return renderLink(token as Tokens.Link, context)
    case 'paragraph':
      return (
        <p className='text-foreground/90 my-4 leading-7'>
          {renderTokens((token as Tokens.Paragraph).tokens, context)}
        </p>
      )
    case 'heading':
      return null
    case 'hr':
      return <hr className='my-8' />
    case 'blockquote':
      return (
        <blockquote className='border-primary/30 text-muted-foreground my-6 border-l-2 pl-5'>
          {renderTokens((token as Tokens.Blockquote).tokens, context)}
        </blockquote>
      )
    case 'list': {
      const list = token as Tokens.List
      const ListElement = list.ordered ? 'ol' : 'ul'
      return (
        <ListElement
          start={list.ordered ? list.start || undefined : undefined}
          className={
            list.ordered
              ? 'my-4 list-decimal space-y-2 pl-6'
              : 'my-4 list-disc space-y-2 pl-6'
          }
        >
          {withStableKeys(list.items, (item) => item.raw).map(
            ({ item, key }) => (
              <li key={key} className='pl-1 leading-7'>
                {renderTokens(item.tokens, context)}
              </li>
            )
          )}
        </ListElement>
      )
    }
    case 'code': {
      const code = token as Tokens.Code
      return (
        <DocsCodeBlock
          code={replaceDocsPlaceholders(code.text, context.placeholders)}
          language={code.lang?.trim().split(/\s+/)[0] ?? 'text'}
        />
      )
    }
    case 'table': {
      const table = token as Tokens.Table
      return (
        <div className='my-6 overflow-x-auto rounded-xl border'>
          <table className='w-full min-w-[36rem] border-collapse text-left text-sm'>
            <thead className='bg-muted/60'>
              <tr>
                {withStableKeys(table.header, (cell) => cell.text).map(
                  ({ item: cell, key }) => (
                    <th key={key} className='border-b px-4 py-3 font-semibold'>
                      {renderTokens(cell.tokens, context)}
                    </th>
                  )
                )}
              </tr>
            </thead>
            <tbody>
              {withStableKeys(table.rows, (row) =>
                row.map((cell) => cell.text).join('\0')
              ).map(({ item: row, key: rowKey }) => (
                <tr key={rowKey} className='border-b last:border-0'>
                  {withStableKeys(row, (cell) => cell.text).map(
                    ({ item: cell, key }) => (
                      <td key={key} className='px-4 py-3 align-top'>
                        {renderTokens(cell.tokens, context)}
                      </td>
                    )
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )
    }
    default:
      return null
  }
}

export function DocsContent({
  document,
  locale,
  manifest,
  placeholders,
}: {
  document: DocsMarkdownDocument
  locale: string
  manifest: DocsManifest
  placeholders: DocsPlaceholderValues
}) {
  const { t } = useTranslation()
  const context = { locale, manifest, placeholders }
  return (
    <div className='docs-content min-w-0'>
      {withStableKeys(document.blocks, (block) => block.token.raw).map(
        ({ item: block, key }) => {
          if (block.token.type !== 'heading') {
            return (
              <Fragment key={key}>{renderToken(block.token, context)}</Fragment>
            )
          }
          const heading = block.token as Tokens.Heading
          const children = renderTokens(heading.tokens, context)
          const id = block.headingId
          if (heading.depth === 1) {
            return (
              <h1
                key={key}
                id={id}
                tabIndex={-1}
                className='scroll-mt-24 text-3xl leading-tight font-semibold tracking-tight sm:text-4xl'
              >
                {children}
              </h1>
            )
          }
          if (heading.depth === 2) {
            return (
              <h2
                key={key}
                id={id}
                tabIndex={-1}
                className='scroll-mt-24 pt-8 text-2xl font-semibold tracking-tight'
              >
                <a href={`#${id}`} className='no-underline'>
                  {children}
                </a>
              </h2>
            )
          }
          return (
            <h3
              key={key}
              id={id}
              tabIndex={-1}
              className='scroll-mt-24 pt-5 text-lg font-semibold'
            >
              <a href={`#${id}`} className='no-underline'>
                {children}
              </a>
            </h3>
          )
        }
      )}
      <p className='text-muted-foreground mt-10 border-t pt-5 text-xs'>
        {t('Last verified: {{date}}', {
          date: document.frontmatter.lastVerified,
        })}
      </p>
    </div>
  )
}
