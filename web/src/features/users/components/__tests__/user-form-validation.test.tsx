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
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import type { User } from '../../types'
import { UsersMutateDrawer } from '../users-mutate-drawer'
import { UsersProvider } from '../users-provider'

const target: User = {
  id: 2,
  username: 'managed-user',
  display_name: 'Managed user',
  email: '',
  role: 1,
  status: 1,
  quota: 0,
  used_quota: 0,
  request_count: 0,
  group: 'default',
}

let client: QueryClient

beforeEach(() => {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'operator', role: 100 })
})

afterEach(() => {
  cleanup()
  client.clear()
  vi.restoreAllMocks()
  useAuthStore.getState().auth.reset()
})

function renderUserForm(currentRow?: User) {
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/group/') {
      return { data: { success: true, data: ['default'] } }
    }
    if (url === '/api/authz/catalog') {
      return {
        data: { success: true, data: { resources: [], roles: [] } },
      }
    }
    if (url === '/api/user/2') {
      return { data: { success: true, data: target } }
    }
    if (url === '/api/verify/methods') {
      return {
        data: {
          success: true,
          data: {
            scope: 'admin.user.update',
            methods: [{ method: '2fa', available: true }],
            oauth_providers: [],
            password_encryption_enabled: false,
          },
        },
      }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
  const post = vi.spyOn(api, 'post').mockImplementation(async (url) => {
    if (url === '/api/verify') {
      return {
        data: {
          success: true,
          data: {
            proof_token: 'update-proof',
            method: '2fa',
            scope: 'admin.user.update',
            expires_at: Math.floor(Date.now() / 1000) + 60,
          },
        },
      }
    }
    if (url === '/api/user/') return { data: { success: true } }
    throw new Error(`Unexpected POST ${url}`)
  })
  const put = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true } })
  const onOpenChange = vi.fn()
  render(
    <QueryClientProvider client={client}>
      <UsersProvider>
        <UsersMutateDrawer
          open
          onOpenChange={onOpenChange}
          currentRow={currentRow}
        />
      </UsersProvider>
    </QueryClientProvider>
  )
  return { user: userEvent.setup(), post, put, onOpenChange }
}

it('resetting a password for a user without email saves only after verification', async () => {
  const { user, post, put, onOpenChange } = renderUserForm(target)
  await screen.findByDisplayValue(target.display_name)
  expect(screen.getByLabelText('Email')).toBeDisabled()
  await user.type(screen.getByLabelText('Password'), 'NewPassword123')
  await user.click(screen.getByRole('button', { name: 'Save changes' }))

  const code = await screen.findByLabelText('Authenticator code or backup code')
  expect(put).not.toHaveBeenCalled()
  expect(screen.queryByText('Email is required')).not.toBeInTheDocument()
  await user.type(code, '123456')
  await user.click(screen.getByRole('button', { name: 'Verify' }))

  await waitFor(() =>
    expect(put).toHaveBeenCalledWith(
      '/api/user/',
      expect.objectContaining({ id: 2, password: 'NewPassword123' }),
      expect.objectContaining({
        headers: { 'X-Security-Proof': 'update-proof' },
        singleUseAuthorization: true,
      })
    )
  )
  expect(put.mock.calls[0][1]).not.toHaveProperty('email')
  expect(post).toHaveBeenCalledWith(
    '/api/verify',
    expect.objectContaining({
      scope: 'admin.user.update',
      context: { user_id: 2 },
    }),
    expect.anything()
  )
  await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
})

it('editing a user without email and leaving password empty preserves credentials', async () => {
  const { user, post, put } = renderUserForm(target)
  const displayName = await screen.findByDisplayValue(target.display_name)
  expect(screen.getByLabelText('Password')).toHaveValue('')
  await user.clear(displayName)
  await user.type(displayName, 'Updated name')
  await user.click(screen.getByRole('button', { name: 'Save changes' }))

  await waitFor(() =>
    expect(put).toHaveBeenCalledWith(
      '/api/user/',
      expect.objectContaining({
        id: 2,
        display_name: 'Updated name',
        password: undefined,
      }),
      {}
    )
  )
  expect(put.mock.calls[0][1]).not.toHaveProperty('email')
  expect(post).not.toHaveBeenCalled()
})

it('editing a user with a short password shows a password error and prevents submission', async () => {
  const { user, post, put } = renderUserForm(target)
  await screen.findByDisplayValue(target.display_name)
  const password = screen.getByLabelText('Password')
  await user.type(password, 'short')
  await user.click(screen.getByRole('button', { name: 'Save changes' }))

  await waitFor(() => expect(password).toHaveAttribute('aria-invalid', 'true'))
  expect(password).toHaveAccessibleDescription(
    'Password must contain between 8 and 128 characters.'
  )
  expect(post).not.toHaveBeenCalled()
  expect(put).not.toHaveBeenCalled()
})

it('creating a user without email shows an error on the editable email field', async () => {
  const { user, post } = renderUserForm()
  const email = screen.getByLabelText('Email')
  expect(email).toBeEnabled()
  await user.type(screen.getByLabelText('Username'), 'new-user')
  await user.type(screen.getByLabelText('Password'), 'NewPassword123')
  await user.click(screen.getByRole('button', { name: 'Save changes' }))

  await waitFor(() => expect(email).toHaveAttribute('aria-invalid', 'true'))
  expect(email).toHaveAccessibleDescription(
    'Required for account notifications Email is required'
  )
  expect(post).not.toHaveBeenCalled()
})

it('creating a user with valid credentials submits the required email', async () => {
  const { user, post, onOpenChange } = renderUserForm()
  await user.type(screen.getByLabelText('Username'), 'new-user')
  await user.type(screen.getByLabelText('Email'), 'new@example.com')
  await user.type(screen.getByLabelText('Password'), 'NewPassword123')
  await user.click(screen.getByRole('button', { name: 'Save changes' }))

  await waitFor(() =>
    expect(post).toHaveBeenCalledWith(
      '/api/user/',
      expect.objectContaining({
        username: 'new-user',
        email: 'new@example.com',
        password: 'NewPassword123',
      }),
      {}
    )
  )
  await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
})

it.each([
  { mode: 'create', currentRow: undefined },
  { mode: 'update', currentRow: target },
])(
  'the $mode password field requests a new password instead of saved credentials',
  async ({ currentRow }) => {
    renderUserForm(currentRow)
    if (currentRow) await screen.findByDisplayValue(currentRow.display_name)

    expect(screen.getByLabelText('Password')).toHaveAttribute(
      'autocomplete',
      'new-password'
    )
  }
)
