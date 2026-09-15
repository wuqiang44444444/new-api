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
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useEffect } from 'react'
import { describe, expect, test, vi } from 'vitest'

import { testChannel } from '../../../api'
import { channelSchema } from '../../../types'
import { ChannelsProvider, useChannels } from '../../channels-provider'
import { ChannelTestDialog } from '../channel-test-dialog'

vi.mock('../../../api', async (importOriginal) => {
  const original = await importOriginal<typeof import('../../../api')>()
  return { ...original, testChannel: vi.fn() }
})

const channel = channelSchema.parse({
  id: 93,
  type: 63,
  key: '',
  status: 1,
  name: 'Image test channel',
  created_time: 0,
  test_time: 0,
  response_time: 0,
  balance_updated_time: 0,
  models: 'nano-banana-2',
})

function SelectedChannelDialog() {
  const { setCurrentRow } = useChannels()
  useEffect(() => {
    setCurrentRow(channel)
  }, [setCurrentRow])
  return <ChannelTestDialog open onOpenChange={() => undefined} />
}

function renderChannelTest() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ChannelsProvider>
        <SelectedChannelDialog />
      </ChannelsProvider>
    </QueryClientProvider>
  )
}

describe('Channel test failure display', () => {
  test('shows the specific billing error instead of claiming the model has no price', async () => {
    const message =
      'the current image adapter cannot supply the verified usage required by this billing expression'
    vi.mocked(testChannel).mockResolvedValue({
      success: false,
      message,
      error_code: 'model_price_error',
    })
    renderChannelTest()
    await userEvent
      .setup()
      .click(await screen.findByRole('button', { name: 'Test Connection' }))
    expect(await screen.findByText(message)).toBeVisible()
    expect(
      screen.queryByText(
        'Model price is not configured. Please complete model pricing in settings.'
      )
    ).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Go to Settings' })).toBeVisible()
    expect(
      screen.queryByRole('button', { name: 'Details' })
    ).not.toBeInTheDocument()
    expect(testChannel).toHaveBeenCalledWith(93, { model: 'nano-banana-2' })
  })

  test('preserves the full multiline error in details without offering pricing settings for a provider error', async () => {
    const firstLine = 'The upstream request failed.'
    const detailLine = 'The provider rejected the requested operation.'
    vi.mocked(testChannel).mockResolvedValue({
      success: false,
      message: `${firstLine}\n${detailLine}`,
      error_code: 'convert_request_failed',
    })
    renderChannelTest()
    const user = userEvent.setup()
    await user.click(
      await screen.findByRole('button', { name: 'Test Connection' })
    )
    expect(await screen.findByText(firstLine)).toBeVisible()
    expect(
      screen.queryByRole('button', { name: 'Go to Settings' })
    ).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Details' }))
    const details = await screen.findByRole('dialog', { name: 'Details' })
    expect(
      within(details).getByText(`${firstLine} ${detailLine}`)
    ).toBeVisible()
    expect(details).toHaveAccessibleDescription('nano-banana-2')
  })

  test('shortens a long summary while keeping the entire billing error in details', async () => {
    const message =
      'The current image adapter cannot supply the verified usage required by this billing expression. The configured expression requires input tokens and image output tokens, and the available usage does not establish both quantities.'
    vi.mocked(testChannel).mockResolvedValue({
      success: false,
      message,
      error_code: 'model_price_error',
    })
    renderChannelTest()
    const user = userEvent.setup()
    await user.click(
      await screen.findByRole('button', { name: 'Test Connection' })
    )
    await screen.findByRole('button', { name: 'Details' })
    expect(screen.queryByText(message)).not.toBeInTheDocument()
    expect(
      screen.getByText(/^The current image adapter.*\.\.\.$/)
    ).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Details' }))
    const details = await screen.findByRole('dialog', { name: 'Details' })
    expect(within(details).getByText(message)).toBeVisible()
  })

  test.each([
    [
      'model_price_error',
      'Model price is not configured. Please complete model pricing in settings.',
    ],
    ['convert_request_failed', 'Test failed'],
  ])(
    'uses the existing fallback when %s contains only whitespace',
    async (code, summary) => {
      vi.mocked(testChannel).mockResolvedValue({
        success: false,
        message: ' \n ',
        error_code: code,
      })
      renderChannelTest()
      await userEvent
        .setup()
        .click(await screen.findByRole('button', { name: 'Test Connection' }))
      expect(await screen.findByText(summary)).toBeVisible()
      expect(
        screen.queryByRole('button', { name: 'Details' })
      ).not.toBeInTheDocument()
    }
  )
})
