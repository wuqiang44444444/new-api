import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { AffinityLayerCacheStats } from '../affinity-layer-cache-stats'

const entriesPath = '/api/option/channel_affinity_cache/entries'

function show() {
  const client = new QueryClient()
  render(
    <QueryClientProvider client={client}>
      <AffinityLayerCacheStats
        onCleared={() => {}}
        stats={{
          enabled: true,
          total: 1,
          unknown: 0,
          by_rule_name: {},
          cache_capacity: 100,
          cache_algo: 'lru',
          api_key_entries: 1,
          user_entries: 0,
        }}
      />
    </QueryClientProvider>
  )
  return userEvent.setup()
}

it.each([
  ['Session', 'session'],
  ['API Key', 'api_key'],
  ['User', 'user'],
])(
  'opens read-only details for %s and restores keyboard focus on close',
  async (label, layer) => {
    const get = vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: {
          entries: [
            {
              fingerprint: '0a1b2c3d4e5f6789',
              channel_id: 42,
              channel_name: 'Primary upstream',
              ttl_seconds: 95,
            },
          ],
        },
      },
    })
    const user = show()
    expect(get).not.toHaveBeenCalled()
    const trigger = screen.getByRole('button', {
      name: `View cache entries for ${label}`,
    })
    trigger.focus()
    await user.keyboard('{Enter}')
    const dialog = await screen.findByRole('dialog', {
      name: `${label} cache entries`,
    })
    expect(
      await within(dialog).findByRole('cell', { name: 'Primary upstream' })
    ).toBeVisible()
    for (const value of ['0a1b2c3d4e5f6789', '42', '95']) {
      expect(within(dialog).getByRole('cell', { name: value })).toBeVisible()
    }
    expect(get).toHaveBeenCalledWith(
      entriesPath,
      expect.objectContaining({ params: { layer, cursor: undefined } })
    )
    expect(
      within(dialog).queryByRole('button', { name: 'Clear cache' })
    ).toBeNull()
    await user.keyboard('{Escape}')
    await waitFor(() => expect(trigger).toHaveFocus())
  }
)

it('retains the next-page action when a scan page is empty', async () => {
  const get = vi
    .spyOn(api, 'get')
    .mockResolvedValueOnce({
      data: { success: true, data: { entries: [], next_cursor: '8' } },
    })
    .mockResolvedValueOnce({ data: { success: true, data: { entries: [] } } })
  const user = show()
  await user.click(
    screen.getByRole('button', { name: 'View cache entries for User' })
  )
  expect(await screen.findByText('No cache entries on this page')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Next page' }))
  expect(await screen.findByText('No cache entries on this page')).toBeVisible()
  expect(get).toHaveBeenLastCalledWith(
    entriesPath,
    expect.objectContaining({ params: { layer: 'user', cursor: '8' } })
  )
  expect(screen.getByRole('button', { name: 'Previous page' })).toBeEnabled()
})

it('shows a read failure and lets Root refresh the selected layer', async () => {
  vi.spyOn(api, 'get')
    .mockRejectedValueOnce(new Error('Offline'))
    .mockResolvedValueOnce({ data: { success: true, data: { entries: [] } } })
  const user = show()
  await user.click(
    screen.getByRole('button', { name: 'View cache entries for API Key' })
  )
  expect(await screen.findByRole('alert')).toHaveTextContent('Loading failed')
  await user.click(screen.getByRole('button', { name: 'Refresh' }))
  expect(await screen.findByText('No cache entries')).toBeVisible()
})
