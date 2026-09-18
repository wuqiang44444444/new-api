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
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import {
  CodeBlock,
  CodeBlockCopyButton,
} from '@/components/ai-elements/code-block'

import { DetailSection } from '../../components/dialogs/log-detail-layout'

type BodySnapshot = { state?: string; body?: string; status?: number }
type Exchange = Record<string, BodySnapshot>

function parseExchange(detail: string): Exchange {
  try {
    const details = JSON.parse(detail || '{}')
    const exchange: unknown = JSON.parse(details.http_exchange || '{}')
    if (!exchange || typeof exchange !== 'object' || Array.isArray(exchange)) {
      return {}
    }
    return exchange as Exchange
  } catch {
    return {}
  }
}

export function ErrorLogHTTPExchange(props: { detail: string }) {
  const { t } = useTranslation()
  const exchange = useMemo(() => parseExchange(props.detail), [props.detail])
  const sections = [
    { key: 'request', label: t('Original request (redacted)') },
    { key: 'upstream_request', label: t('Upstream request (redacted)') },
    { key: 'upstream_response', label: t('Upstream response (redacted)') },
    { key: 'response', label: t('Gateway response (redacted)') },
  ]
  return (
    <div className='min-w-0 space-y-3'>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Sensitive fields and URLs are redacted. Long text may be shortened; oversized and non-JSON bodies are not recorded.'
        )}
      </p>
      {sections.map((section) => {
        const snapshot = exchange[section.key]
        let unavailable = t('Not recorded for this event')
        if (snapshot?.state === 'too_large') {
          unavailable = t('Body exceeds the diagnostic size limit')
        }
        if (snapshot?.state === 'unsupported') {
          unavailable = t('Non-JSON or malformed body was not recorded')
        }
        if (snapshot?.state === 'read_failed') {
          unavailable = t('Body could not be read completely')
        }
        if (snapshot?.state === 'empty') unavailable = t('Empty body')
        // The server formats JSON without rounding large numeric identifiers.
        const body = snapshot?.body
        return (
          <DetailSection key={section.key} label={section.label}>
            {typeof snapshot?.status === 'number' && snapshot.status > 0 && (
              <p className='text-muted-foreground text-xs'>
                HTTP {snapshot.status}
              </p>
            )}
            {snapshot?.state === 'captured' && typeof body === 'string' ? (
              <CodeBlock
                code={body}
                language='json'
                showToolbar
                defaultCollapsed
                maxExpandedLines={24}
              >
                <CodeBlockCopyButton />
              </CodeBlock>
            ) : (
              <p className='text-muted-foreground text-xs'>{unavailable}</p>
            )}
          </DetailSection>
        )
      })}
    </div>
  )
}
