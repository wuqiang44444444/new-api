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
import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { STATUS_QUERY_KEY, STATUS_STORAGE_KEY } from '@/lib/status-query'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { Home } from '..'

let queryClient: QueryClient

beforeEach(() => {
  localStorage.clear()
  useAuthStore.getState().auth.reset()
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
})

afterEach(() => {
  cleanup()
  queryClient.clear()
  localStorage.clear()
  useAuthStore.getState().auth.reset()
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
})

async function renderHome(statusRequest?: Promise<{ data: unknown }>) {
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    switch (url) {
      case '/api/status':
        if (statusRequest) return statusRequest
        throw new Error('Expected status to be in the query cache')
      case '/api/notice':
      case '/api/home_page_content':
        return { data: { success: true, data: '' } }
      default:
        throw new Error(`Unexpected GET ${url}`)
    }
  })

  const router = createRouter({
    routeTree: createRootRoute({ component: Home }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })

  await router.load()
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  await screen.findByRole('heading', { name: /Reliable AI model access/ })
}

test('shows model square navigation and both pricing buttons by default', async () => {
  queryClient.setQueryData(STATUS_QUERY_KEY, {})
  await renderHome()

  const header = within(screen.getByRole('banner'))
  expect(header.getByRole('link', { name: 'Model Square' })).toBeVisible()
  expect(screen.getAllByRole('button', { name: 'View Pricing' })).toHaveLength(
    2
  )
})

test('hides model square links from guests when login is required', async () => {
  queryClient.setQueryData(STATUS_QUERY_KEY, {
    HeaderNavModules: '{"pricing":{"enabled":true,"requireAuth":true}}',
  })
  await renderHome()
  await userEvent.click(
    screen.getByRole('button', { name: 'Toggle navigation menu' })
  )

  expect(screen.queryAllByRole('link', { name: 'Model Square' })).toHaveLength(
    0
  )
  expect(
    screen.queryAllByRole('button', { name: 'View Pricing' })
  ).toHaveLength(0)
})

test('shows model square navigation to a signed-in user when login is required', async () => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'member', role: 1 })
  queryClient.setQueryData(STATUS_QUERY_KEY, {
    HeaderNavModules: '{"pricing":{"enabled":true,"requireAuth":true}}',
  })
  await renderHome()

  const header = within(screen.getByRole('banner'))
  expect(header.getByRole('link', { name: 'Model Square' })).toBeVisible()
})

test('keeps pricing out of the home fallback navigation and buttons when disabled', async () => {
  queryClient.setQueryData(STATUS_QUERY_KEY, {
    HeaderNavModules: {
      pricing: false,
      home: false,
      console: false,
      rankings: false,
      docs: false,
      about: false,
    },
  })
  await renderHome()

  const header = within(screen.getByRole('banner'))
  expect(header.getByRole('link', { name: 'Models' })).toBeVisible()
  expect(
    header.queryByRole('link', { name: 'Pricing' })
  ).not.toBeInTheDocument()
  expect(
    screen.queryAllByRole('button', { name: 'View Pricing' })
  ).toHaveLength(0)
})

test('withholds model square links while an old public status snapshot is being refreshed', async () => {
  localStorage.setItem(STATUS_STORAGE_KEY, JSON.stringify({}))
  const statusRequest = new Promise<{ data: unknown }>(() => {})
  await renderHome(statusRequest)

  expect(screen.queryAllByRole('link', { name: 'Model Square' })).toHaveLength(
    0
  )
  expect(
    screen.queryAllByRole('button', { name: 'View Pricing' })
  ).toHaveLength(0)
})
