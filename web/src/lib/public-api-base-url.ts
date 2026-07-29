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
import type { SystemStatus } from '@/features/auth/types'

function publicStatusValue(
  status: SystemStatus | null,
  field: 'server_address' | 'system_name'
): string {
  const direct = status?.[field]
  const nested = status?.data?.[field]
  if (typeof direct === 'string') return direct
  if (typeof nested === 'string') return nested
  return ''
}

export function resolvePublicSiteBaseUrl(
  status: SystemStatus | null,
  requestOrigin = typeof window === 'undefined' ? '' : window.location.origin
): string {
  const configured = publicStatusValue(status, 'server_address').trim()

  for (const candidate of [configured, requestOrigin]) {
    if (!candidate) continue
    try {
      const url = new URL(candidate)
      if (
        (url.protocol === 'http:' || url.protocol === 'https:') &&
        !url.username &&
        !url.password
      ) {
        const pathname = url.pathname.replace(/\/+$/, '').replace(/\/v1$/i, '')
        url.pathname = pathname || '/'
        url.search = ''
        url.hash = ''
        return url.href.replace(/\/+$/, '')
      }
    } catch {
      // Try the next public fallback.
    }
  }

  return ''
}

export function resolvePublicSystemName(status: SystemStatus | null): string {
  return publicStatusValue(status, 'system_name').trim() || 'API'
}

export function resolveOpenAIBaseUrl(
  status: SystemStatus | null,
  requestOrigin?: string
): string {
  const siteBase = resolvePublicSiteBaseUrl(status, requestOrigin)
  return siteBase ? `${siteBase}/v1` : '/v1'
}
