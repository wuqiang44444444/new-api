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
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { SummaryCards } from '../summary-cards'

async function renderSummaryCards() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const rootRoute = createRootRoute({
    component: () => (
      <QueryClientProvider client={client}>
        <SummaryCards />
      </QueryClientProvider>
    ),
  })
  const router = createRouter({
    routeTree: rootRoute,
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  return render(<RouterProvider router={router} />)
}

function setStoredUser(user: Record<string, unknown>) {
  useAuthStore.getState().auth.setUser(user as never)
}

describe('SummaryCards usage card display', () => {
  beforeEach(() => {
    useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      if (url === '/api/user/self') {
        throw new Error('network down')
      }
      return { data: { success: true, data: [] } }
    })
  })

  afterEach(() => {
    useAuthStore.getState().auth.reset()
    vi.restoreAllMocks()
  })

  it('withholds an unresolved amount instead of displaying stale usage or zero', async () => {
    setStoredUser({ id: 1, username: 'randy', role: 1, used_quota: null })
    await renderSummaryCards()
    expect(await screen.findByText('Usage needs review')).toBeInTheDocument()
    expect(screen.queryByText('$11,090.31')).not.toBeInTheDocument()
  })

  it('renders the stored net consumption with the refunds-deducted description', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      if (url === '/api/user/self') {
        return {
          data: { success: true, data: { id: 1, used_quota: 2829317796 } },
        }
      }
      return { data: { success: true, data: [] } }
    })
    setStoredUser({ id: 1, username: 'randy', role: 1, used_quota: 2829317796 })
    await renderSummaryCards()
    await waitFor(() => {
      expect(screen.getByText(/5,658\.64/)).toBeInTheDocument()
    })
    expect(
      screen.getByText(
        'Refunds deducted; in-progress task charges update after settlement (USD)'
      )
    ).toBeInTheDocument()
  })

  it('shows unknown instead of zero when the cumulative value is absent', async () => {
    setStoredUser({ id: 1, username: 'randy', role: 1 })
    await renderSummaryCards()
    await waitFor(() => {
      expect(screen.getByText('Usage needs review')).toBeInTheDocument()
    })
  })

  it('keeps the last value and explains the stale refresh when refresh fails', async () => {
    setStoredUser({ id: 1, username: 'randy', role: 1, used_quota: 2829317796 })
    await renderSummaryCards()
    await waitFor(() => {
      expect(
        screen.getByText('Refresh failed, showing last synced value (USD)')
      ).toBeInTheDocument()
    })
    // The previous value must stay visible; a failed refresh never zeroes it.
    expect(screen.getByText(/5,658\.64/)).toBeInTheDocument()
  })
})
