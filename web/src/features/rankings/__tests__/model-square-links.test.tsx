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
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, expect, it } from 'vitest'

import { STATUS_QUERY_KEY } from '@/lib/status-query'
import { useAuthStore } from '@/stores/auth-store'

import { ModelLink, VendorLink } from '../components/entity-links'

afterEach(() => {
  cleanup()
  useAuthStore.getState().auth.reset()
})

it.each([
  { enabled: true, requireAuth: false, signedIn: false, links: true },
  { enabled: true, requireAuth: true, signedIn: false, links: false },
  { enabled: true, requireAuth: true, signedIn: true, links: true },
  { enabled: false, requireAuth: false, signedIn: false, links: false },
])(
  'uses the model-square access policy on ranking links: %j',
  async (access) => {
    if (access.signedIn) {
      useAuthStore
        .getState()
        .auth.setUser({ id: 1, username: 'member', role: 1 })
    }
    const client = new QueryClient()
    client.setQueryData(STATUS_QUERY_KEY, {
      HeaderNavModules: JSON.stringify({ pricing: access }),
    })
    const router = createRouter({
      routeTree: createRootRoute({
        component: () => (
          <>
            <ModelLink modelName='image-model' />
            <VendorLink vendor='Example' />
          </>
        ),
      }),
      history: createMemoryHistory({ initialEntries: ['/'] }),
    })
    await router.load()
    render(
      <QueryClientProvider client={client}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    )
    await screen.findByText('image-model')
    expect(screen.getByText('Example')).toBeVisible()
    expect(screen.queryAllByRole('link')).toHaveLength(access.links ? 2 : 0)
  }
)
