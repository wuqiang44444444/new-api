/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { useState } from 'react'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { beforeAll, describe, expect, it } from 'vitest'

import type { ProviderChannelSummary } from '../../types'
import { UpstreamStatementTable } from '../upstream-statement-table'

const i18n = createInstance()

beforeAll(async () => {
  await i18n.use(initReactI18next).init({
    lng: 'en',
    resources: { en: { translation: {} } },
  })
})

const channel: ProviderChannelSummary = {
  channel_id: 18,
  channel_name: 'Channel 18',
  usage: {
    requests: 1,
    billable_calls: 0,
    input_tokens: 245,
    cache_read_tokens: 0,
    cache_write_tokens: 0,
    output_tokens: 385,
  },
  models: [
    {
      channel_id: 18,
      channel_name: 'Channel 18',
      provider_model: 'gpt-5.6-sol',
      billing_mode: 'token',
      usage: {
        requests: 1,
        billable_calls: 0,
        input_tokens: 245,
        cache_read_tokens: 0,
        cache_write_tokens: 0,
        output_tokens: 385,
      },
      discount: { value: '1', version: 1, source: 'default' },
      data_quality: { status: 'complete' },
      detail_filter: {
        start_timestamp: 1,
        end_timestamp: 2,
        channel_id: 18,
        model_name: 'gpt-5.6-sol',
      },
    },
  ],
  data_quality: { status: 'complete' },
}

function TableHarness() {
  const [expanded, setExpanded] = useState(new Set<number>())
  return (
    <I18nextProvider i18n={i18n}>
      <UpstreamStatementTable
        channels={[channel]}
        expandedChannels={expanded}
        onToggleChannel={(channelId) =>
          setExpanded((current) => {
            const next = new Set(current)
            if (next.has(channelId)) next.delete(channelId)
            else next.add(channelId)
            return next
          })
        }
      />
    </I18nextProvider>
  )
}

describe('upstream statement hierarchy', () => {
  it('expands a channel parent into one model row with accessible state', async () => {
    render(<TableHarness />)
    const user = userEvent.setup()
    const toggle = screen.getByRole('button', { name: 'Expand models' })

    expect(screen.queryByText('Supplier statement')).toBeNull()
    expect(screen.queryByText('Reference amount')).toBeNull()
    expect(toggle.getAttribute('aria-expanded')).toBe('false')
    expect(screen.queryByText('gpt-5.6-sol')).toBeNull()

    await user.click(toggle)

    expect(
      screen
        .getByRole('button', { name: 'Collapse models' })
        .getAttribute('aria-expanded')
    ).toBe('true')
    expect(screen.getAllByText('gpt-5.6-sol')).toHaveLength(1)
    const modelRow = screen.getByText('gpt-5.6-sol').closest('tr')
    expect(modelRow).not.toBeNull()
    expect(
      within(modelRow as HTMLTableRowElement).getByText('245')
    ).toBeTruthy()
    expect(
      within(modelRow as HTMLTableRowElement).getByRole('link', {
        name: 'View details',
      })
    ).toBeTruthy()
  })

  it('shows the concrete reasons when a partial-data badge is hovered', async () => {
    const partialChannel: ProviderChannelSummary = {
      ...channel,
      data_quality: {
        status: 'partial',
        unknown_billing_mode_requests: 7,
      },
    }
    const user = userEvent.setup()
    render(
      <I18nextProvider i18n={i18n}>
        <UpstreamStatementTable
          channels={[partialChannel]}
          expandedChannels={new Set()}
          onToggleChannel={() => undefined}
        />
      </I18nextProvider>
    )

    await user.hover(screen.getByText('Partial data'))

    expect(await screen.findByText('Partial data reasons')).toBeTruthy()
    expect(
      screen.getByText('7 records do not contain a frozen billing mode.')
    ).toBeTruthy()
  })
})
