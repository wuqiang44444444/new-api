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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { beforeAll, describe, expect, it, vi } from 'vitest'

import { LinkPublicationRebindDialog } from '../link-publication-rebind-dialog'

const { rebindLinkModelPublication } = vi.hoisted(() => ({
  rebindLinkModelPublication: vi.fn(),
}))

vi.mock('../../../api', () => ({ rebindLinkModelPublication }))
vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

beforeAll(async () => {
  await i18next.use(initReactI18next).init({
    lng: 'en',
    resources: { en: { translation: {} } },
  })
})

describe('Link publication rebind dialog', () => {
  it('requires a reason and submits the immutable key with expected version', async () => {
    rebindLinkModelPublication.mockResolvedValue({ success: true })
    const onOpenChange = vi.fn()
    const queryClient = new QueryClient({
      defaultOptions: { mutations: { retry: false } },
    })

    render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18next}>
          <LinkPublicationRebindDialog
            open
            onOpenChange={onOpenChange}
            publication={{
              id: 1,
              contract_namespace: 'link',
              route_family: 'modelark_video',
              customer_model: 'customer-video-v1',
              link_sku: 'old-sku',
              publication_version: 7,
              source_channel_id: 10,
              change_reason: 'publish',
              created_at: 1,
              updated_at: 1,
              currently_fulfillable: true,
              routing_conflict: false,
            }}
            proposedSKU='new-sku'
          />
        </I18nextProvider>
      </QueryClientProvider>
    )

    const confirmButton = screen.getByRole('button', {
      name: 'Confirm rebind',
    })
    expect((confirmButton as HTMLButtonElement).disabled).toBe(true)

    fireEvent.change(screen.getByLabelText('Rebind reason'), {
      target: { value: 'provider contract migration' },
    })
    fireEvent.click(confirmButton)

    await waitFor(() => {
      expect(rebindLinkModelPublication.mock.calls[0]?.[0]).toEqual({
        contract_namespace: 'link',
        route_family: 'modelark_video',
        customer_model: 'customer-video-v1',
        link_sku: 'new-sku',
        expected_version: 7,
        reason: 'provider contract migration',
      })
      expect(onOpenChange).toHaveBeenCalledWith(false)
    })

    queryClient.clear()
  })
})
