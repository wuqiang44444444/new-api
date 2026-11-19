import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { CommonLogsStats } from '../common-logs-stats'
import { UsageLogsProvider } from '../usage-logs-provider'

const route = vi.hoisted(() => ({ search: { model: 'first' } }))
vi.mock('@tanstack/react-router', () => ({
  getRouteApi: () => ({ useSearch: () => route.search }),
}))

const client = new QueryClient()
afterEach(() => {
  cleanup()
  client.clear()
  vi.restoreAllMocks()
  route.search = { model: 'first' }
})

it('requests statistics only on click for each filter and cancels stale work', async () => {
  let firstSignal: AbortSignal | undefined
  let firstStarted!: () => void
  const started = new Promise<void>((resolve) => {
    firstStarted = resolve
  })
  const request = vi
    .spyOn(api, 'get')
    .mockImplementation(async (url, config) => {
      if (url.includes('model_name=first')) {
        firstSignal = config?.signal as AbortSignal
        firstStarted()
        await new Promise<void>((resolve) => {
          firstSignal?.addEventListener('abort', () => resolve(), {
            once: true,
          })
        })
      }
      return {
        data: { success: true, data: { quota: 0, rpm: 0, tpm: 0, total: 3 } },
      }
    })
  const component = () => (
    <QueryClientProvider client={client}>
      <UsageLogsProvider>
        <CommonLogsStats window={{ start: 100, end: 200 }} />
      </UsageLogsProvider>
    </QueryClientProvider>
  )
  const { rerender } = render(component())
  expect(request).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole('button', { name: 'View range statistics' }))
  await started
  route.search = { model: 'second' }
  await act(async () => rerender(component()))
  expect(firstSignal?.aborted).toBe(true)
  expect(request).toHaveBeenCalledTimes(1)
  fireEvent.click(screen.getByRole('button', { name: 'View range statistics' }))
  expect(await screen.findByText('Total Count')).toBeVisible()
  expect(request).toHaveBeenCalledTimes(2)
})

it('waits for a manual retry after a failed statistics request', async () => {
  const request = vi
    .spyOn(api, 'get')
    .mockResolvedValueOnce({ data: { success: false, message: 'Unavailable' } })
    .mockResolvedValueOnce({
      data: { success: true, data: { quota: 0, rpm: 0, tpm: 0, total: 3 } },
    })
  render(
    <QueryClientProvider client={client}>
      <UsageLogsProvider>
        <CommonLogsStats window={{ start: 100, end: 200 }} />
      </UsageLogsProvider>
    </QueryClientProvider>
  )
  fireEvent.click(screen.getByRole('button', { name: 'View range statistics' }))
  const retry = await screen.findByRole('button', { name: 'Retry' })
  expect(request).toHaveBeenCalledTimes(1)
  fireEvent.click(retry)
  expect(await screen.findByText('Total Count')).toBeVisible()
  expect(request).toHaveBeenCalledTimes(2)
})
