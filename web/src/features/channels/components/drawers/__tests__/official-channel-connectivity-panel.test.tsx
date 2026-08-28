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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { I18nextProvider } from 'react-i18next'
import { beforeAll, describe, expect, test, vi } from 'vitest'

import { Form } from '@/components/ui/form'

import {
  createOrReuseChannelDefaultAssetGroup,
  getChannelDefaultAssetGroup,
} from '../../../api'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  type ChannelFormValues,
} from '../../../lib/channel-form'
import { OfficialChannelConnectivityPanel } from '../official-channel-connectivity-panel'

vi.mock('../../../api', async (importOriginal) => {
  const original = await importOriginal<typeof import('../../../api')>()
  return {
    ...original,
    createOrReuseChannelDefaultAssetGroup: vi.fn(),
    getChannelDefaultAssetGroup: vi.fn(),
  }
})

vi.mock('sonner', () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}))

beforeAll(async () => {
  await i18next.init({
    lng: 'en',
    fallbackLng: 'en',
    resources: { en: { translation: {} } },
  })
})

function ConnectivityPanelHarness() {
  const form = useForm<ChannelFormValues>({
    defaultValues: {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      type: 61,
      video_upstream_protocol: 'modelark_v3_volcengine',
      asset_upstream_protocol: 'volcengine_assets_action_v2024_01_01',
    },
  })
  const [queryClient] = useState(
    () =>
      new QueryClient({
        defaultOptions: { queries: { retry: false } },
      })
  )

  return (
    <I18nextProvider i18n={i18next}>
      <QueryClientProvider client={queryClient}>
        <Form {...form}>
          <OfficialChannelConnectivityPanel
            channelId={27}
            control={form.control}
            credentialConfigured
            savedAssetProtocol='volcengine_assets_action_v2024_01_01'
            savedVideoProtocol='modelark_v3_volcengine'
            sensitiveLocked={false}
            onCredentialCleared={vi.fn()}
          />
        </Form>
      </QueryClientProvider>
    </I18nextProvider>
  )
}

describe('Official channel default asset group', () => {
  test('shows the action below connectivity tests and refreshes configured status', async () => {
    vi.mocked(getChannelDefaultAssetGroup).mockResolvedValue({
      success: true,
      data: {
        supported: true,
        configured: false,
        name: 'aigctokenaigeneral',
      },
    })
    vi.mocked(createOrReuseChannelDefaultAssetGroup).mockResolvedValue({
      success: true,
      data: {
        supported: true,
        configured: true,
        name: 'aigctokenaigeneral',
        action: 'created',
      },
    })
    const user = userEvent.setup()

    render(<ConnectivityPanelHarness />)

    const testButton = screen.getByRole('button', { name: 'Test Asset Action' })
    const groupButton = await screen.findByRole('button', {
      name: 'Create or reuse default asset group',
    })
    expect(await screen.findByText('Not configured')).toBeTruthy()
    expect(
      testButton.compareDocumentPosition(groupButton) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()

    await user.click(groupButton)

    await waitFor(() => {
      expect(createOrReuseChannelDefaultAssetGroup).toHaveBeenCalledWith(27)
      expect(screen.getByText('Configured')).toBeTruthy()
    })
  })
})
