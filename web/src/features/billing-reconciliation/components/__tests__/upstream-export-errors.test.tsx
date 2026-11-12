import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import axios from 'axios'
import { toast } from 'sonner'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import i18n from '@/i18n/config'
import { api } from '@/lib/http-client'

import { UpstreamDetailPanel } from '../upstream-detail-panel'

vi.mock('../../api', () => ({
  getAdminUpstreamDetails: vi.fn().mockResolvedValue({
    success: true,
    data: { result: { items: [], total: 1 } },
  }),
}))
vi.mock('../../export-jobs-drawer', () => ({
  ExportJobsDrawer: (props: { open: boolean; children: React.ReactNode }) =>
    props.open ? props.children : null,
}))
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

const originalAdapter = api.defaults.adapter
const originalLanguage = i18n.language
beforeEach(async () => {
  await i18n.changeLanguage('en')
})
afterEach(async () => {
  api.defaults.adapter = originalAdapter
  await i18n.changeLanguage(originalLanguage)
})

it.each([
  [200, 'upstream details require a URL or channel filter'],
  [400, 'billing export must cover one natural month in Asia/Shanghai'],
  [
    409,
    'You already have an active export job. Wait for it to finish or cancel it.',
  ],
  [503, 'Export requires object storage to be configured.'],
] as const)(
  'shows one specific error for upstream export response %s',
  async (status, message) => {
    api.defaults.adapter = async (config) => {
      const response = {
        config,
        status,
        statusText: 'Error',
        headers: {},
        data: { success: false, message },
      }
      if (status >= 400) {
        throw new axios.AxiosError(
          `Request failed with status code ${status}`,
          'ERR_BAD_REQUEST',
          config,
          undefined,
          response
        )
      }
      return response
    }
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={queryClient}>
        <UpstreamDetailPanel
          period={{ start_timestamp: 1788192000, end_timestamp: 1790783999 }}
          selection={{ channelId: 7 }}
          onClose={vi.fn()}
        />
      </QueryClientProvider>
    )
    fireEvent.click(screen.getByRole('button', { name: 'Export and download' }))
    const exportButton = screen.getByRole('button', {
      name: 'Export CSV',
    })
    await waitFor(() => expect(exportButton).toBeEnabled())
    fireEvent.click(exportButton)
    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledExactlyOnceWith(message)
    )
    expect(screen.getByRole('button', { name: 'Export CSV' })).toBeEnabled()
    queryClient.clear()
  }
)
