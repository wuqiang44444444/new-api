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
import {
  resolveOpenAIBaseUrl,
  resolvePublicSiteBaseUrl,
  resolvePublicSystemName,
} from '@/lib/public-api-base-url'

import type { DocsPlaceholderValues } from '../types'

const placeholderPattern = /\{\{([A-Z0-9_]+)\}\}/g

export function docsPlaceholderValues(
  status: SystemStatus | null,
  requestOrigin?: string
): DocsPlaceholderValues {
  const siteBase = resolvePublicSiteBaseUrl(status, requestOrigin)
  return {
    SYSTEM_NAME: resolvePublicSystemName(status),
    SITE_BASE_URL: siteBase,
    OPENAI_BASE_URL: resolveOpenAIBaseUrl(status, requestOrigin),
    ANTHROPIC_BASE_URL: siteBase,
    API_KEY_PLACEHOLDER: 'sk-your-key',
    MODEL_ID_PLACEHOLDER: 'available-model-id',
  }
}

export function replaceDocsPlaceholders(
  value: string,
  placeholders: DocsPlaceholderValues
): string {
  return value.replace(placeholderPattern, (_, key: string) => {
    if (!(key in placeholders)) {
      throw new Error(`Unknown documentation placeholder: ${key}`)
    }
    return placeholders[key as keyof DocsPlaceholderValues]
  })
}
