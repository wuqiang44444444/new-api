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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import i18next from 'i18next'
import { initReactI18next, I18nextProvider } from 'react-i18next'
import { beforeAll, describe, expect, it, vi } from 'vitest'

import { DocsCodeBlock } from '../components/docs-code-block'

vi.mock('shiki', () => ({
  codeToTokensWithThemes: async (code: string) => [
    [
      {
        content: code,
        offset: 0,
        variants: {
          dark: { color: '#ffffff' },
          light: { color: '#000000' },
        },
      },
    ],
  ],
}))

beforeAll(async () => {
  await i18next.use(initReactI18next).init({
    lng: 'en',
    resources: {
      en: {
        translation: {
          'Plain text': 'Plain text',
          Copied: 'Copied',
          'Copy code': 'Copy code',
          Copy: 'Copy',
        },
      },
    },
  })
})

describe('documentation code block', () => {
  it('copies the original code and exposes copy feedback', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    })
    render(
      <I18nextProvider i18n={i18next}>
        <DocsCodeBlock code='curl "https://example.com"' language='bash' />
      </I18nextProvider>
    )

    fireEvent.click(screen.getByRole('button', { name: 'Copy code' }))

    await waitFor(() => {
      expect(writeText).toHaveBeenCalledWith('curl "https://example.com"')
      expect(screen.getByRole('button', { name: 'Copied' })).toBeTruthy()
    })
  })
})
