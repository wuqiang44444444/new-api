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
import { act, render } from '@testing-library/react'
import {
  afterEach,
  beforeEach,
  describe,
  expect,
  it,
  vi,
  type MockInstance,
} from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore, type AuthUser } from '@/stores/auth-store'

import { useUserProfileRefresh } from '../use-user-profile-refresh'

function deferredResponse<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: unknown) => void
  const promise = new Promise<T>((finish, fail) => {
    resolve = finish
    reject = fail
  })
  return { promise, resolve, reject }
}

function selfResponse(user: Partial<AuthUser>) {
  return { data: { success: true, data: user } }
}

function seedSession(sid: string) {
  useAuthStore.getState().auth.setBundle({
    access_token: 'token',
    token_type: 'Bearer',
    access_expires_at: Date.now() + 60_000,
    user: { id: 1, username: 'randy', role: 1, used_quota: 1000 },
    session: {
      sid,
      current: true,
      login_method: 'password',
      ip: '',
      user_agent: '',
      created_at: 1,
      last_active_at: 1,
      expires_at: 2,
    },
  })
}

function Harness() {
  const { refreshFailed } = useUserProfileRefresh()
  return (
    <output aria-label='refresh-state'>
      {refreshFailed ? 'failed' : 'ok'}
    </output>
  )
}

function renderHarness() {
  return render(<Harness />)
}

async function flush() {
  await act(async () => {})
}

describe('useUserProfileRefresh', () => {
  let getSpy: MockInstance

  beforeEach(() => {
    getSpy = vi.spyOn(api, 'get')
  })

  afterEach(() => {
    useAuthStore.getState().auth.reset()
    vi.restoreAllMocks()
  })

  it('writes a fresh self response back to the auth store on mount', async () => {
    seedSession('sid-1')
    getSpy.mockResolvedValue(
      selfResponse({
        id: 1,
        username: 'randy',
        role: 1,
        used_quota: 2829317796,
      })
    )
    renderHarness()
    await flush()
    expect(useAuthStore.getState().auth.user?.used_quota).toBe(2829317796)
  })

  it('keeps the stored value and flags failure when the refresh fails', async () => {
    seedSession('sid-1')
    getSpy.mockRejectedValue(new Error('network down'))
    const view = renderHarness()
    await flush()
    expect(useAuthStore.getState().auth.user?.used_quota).toBe(1000)
    expect(view.getByLabelText('refresh-state').textContent).toBe('failed')
  })

  it('ignores a response whose session was replaced by a re-login', async () => {
    seedSession('sid-1')
    const deferred = deferredResponse<ReturnType<typeof selfResponse>>()
    getSpy.mockReturnValueOnce(deferred.promise as never)
    renderHarness()
    await flush()

    // Same user re-logs-in under a new session while the request is in flight.
    act(() => {
      useAuthStore.getState().auth.setBundle({
        access_token: 'token-2',
        token_type: 'Bearer',
        access_expires_at: Date.now() + 60_000,
        user: { id: 1, username: 'randy', role: 1, used_quota: 100 },
        session: {
          sid: 'sid-2',
          current: true,
          login_method: 'password',
          ip: '',
          user_agent: '',
          created_at: 2,
          last_active_at: 2,
          expires_at: 3,
        },
      })
    })
    await act(async () => {
      deferred.resolve(selfResponse({ id: 1, used_quota: 999 }))
      await deferred.promise
    })
    expect(useAuthStore.getState().auth.user?.used_quota).toBe(100)
    expect(useAuthStore.getState().auth.session?.sid).toBe('sid-2')
  })

  it('ignores a response captured under a different account', async () => {
    seedSession('sid-1')
    const deferred = deferredResponse<ReturnType<typeof selfResponse>>()
    getSpy.mockReturnValueOnce(deferred.promise as never)
    renderHarness()
    await flush()

    act(() => {
      useAuthStore
        .getState()
        .auth.setUser({ id: 2, username: 'other', role: 1, used_quota: 55 })
    })
    await act(async () => {
      deferred.resolve(selfResponse({ id: 1, used_quota: 999 }))
      await deferred.promise
    })
    expect(useAuthStore.getState().auth.user?.id).toBe(2)
    expect(useAuthStore.getState().auth.user?.used_quota).toBe(55)
  })

  it('ignores a server response that answers another identity', async () => {
    seedSession('sid-1')
    getSpy.mockResolvedValue(
      selfResponse({ id: 99, username: 'mix', role: 1, used_quota: 7 })
    )
    renderHarness()
    await flush()
    expect(useAuthStore.getState().auth.user?.id).toBe(1)
    expect(useAuthStore.getState().auth.user?.used_quota).toBe(1000)
  })

  it('refetches for the new identity after an account switch', async () => {
    seedSession('sid-1')
    getSpy.mockResolvedValue(
      selfResponse({ id: 1, username: 'randy', role: 1, used_quota: 10 })
    )
    render(<Harness />)
    await flush()

    getSpy.mockResolvedValue(
      selfResponse({ id: 2, username: 'other', role: 1, used_quota: 66 })
    )
    act(() => {
      useAuthStore
        .getState()
        .auth.setUser({ id: 2, username: 'other', role: 1, used_quota: 55 })
    })
    await flush()
    const calls = getSpy.mock.calls.filter(
      (call: unknown[]) => call[0] === '/api/user/self'
    )
    expect(calls.length).toBeGreaterThanOrEqual(2)
    expect(useAuthStore.getState().auth.user?.used_quota).toBe(66)
  })

  it('bounds focus bursts to the refresh interval', async () => {
    let now = 1_000_000
    const dateSpy = vi.spyOn(Date, 'now')
    dateSpy.mockImplementation(() => now)
    seedSession('sid-1')
    getSpy.mockResolvedValue(
      selfResponse({ id: 1, username: 'randy', role: 1, used_quota: 20 })
    )
    renderHarness()
    await flush()
    expect(selfCalls()).toBe(1)

    act(() => {
      window.dispatchEvent(new Event('focus'))
      window.dispatchEvent(new Event('focus'))
    })
    await flush()
    expect(selfCalls()).toBe(1)

    now += 61_000
    act(() => {
      window.dispatchEvent(new Event('focus'))
    })
    await flush()
    expect(selfCalls()).toBe(2)
  })

  it('ignores an old account failure after the new account refreshed', async () => {
    seedSession('sid-1')
    const oldRequest = deferredResponse<ReturnType<typeof selfResponse>>()
    getSpy.mockReturnValueOnce(oldRequest.promise)
    const view = renderHarness()
    await flush()
    getSpy.mockResolvedValue(selfResponse({ id: 2, used_quota: 55 }))
    act(() => {
      useAuthStore
        .getState()
        .auth.setUser({ id: 2, username: 'other', role: 1, used_quota: 10 })
    })
    await flush()
    await act(async () => {
      oldRequest.reject(new Error('old request failed'))
    })
    expect(view.getByLabelText('refresh-state')).toHaveTextContent('ok')
    expect(useAuthStore.getState().auth.user?.used_quota).toBe(55)
  })

  it('keeps the new account request in flight when the old request finishes', async () => {
    let now = 1_000_000
    vi.spyOn(Date, 'now').mockImplementation(() => now)
    seedSession('sid-1')
    const oldRequest = deferredResponse<ReturnType<typeof selfResponse>>()
    const newRequest = deferredResponse<ReturnType<typeof selfResponse>>()
    getSpy
      .mockReturnValueOnce(oldRequest.promise)
      .mockReturnValueOnce(newRequest.promise)
    renderHarness()
    await flush()
    act(() => {
      useAuthStore
        .getState()
        .auth.setUser({ id: 2, username: 'other', role: 1, used_quota: 10 })
    })
    await flush()
    await act(async () => {
      oldRequest.reject(new Error('old request failed'))
    })
    now += 61_000
    act(() => {
      window.dispatchEvent(new Event('focus'))
    })
    await flush()
    expect(selfCalls()).toBe(2)
    await act(async () => {
      newRequest.resolve(selfResponse({ id: 2, used_quota: 55 }))
    })
    expect(useAuthStore.getState().auth.user?.used_quota).toBe(55)
  })

  it('bounds focus retries after failures as well as successes', async () => {
    let now = 1_000_000
    vi.spyOn(Date, 'now').mockImplementation(() => now)
    seedSession('sid-1')
    getSpy.mockRejectedValue(new Error('unavailable'))
    renderHarness()
    await flush()
    act(() => {
      window.dispatchEvent(new Event('focus'))
    })
    await flush()
    act(() => {
      document.dispatchEvent(new Event('visibilitychange'))
    })
    await flush()
    expect(selfCalls()).toBe(1)
    now += 61_000
    act(() => {
      window.dispatchEvent(new Event('focus'))
    })
    await flush()
    expect(selfCalls()).toBe(2)
    expect(useAuthStore.getState().auth.user?.used_quota).toBe(1000)
  })

  it('starts a fresh request immediately when the same user changes session', async () => {
    seedSession('sid-1')
    const oldRequest = deferredResponse<ReturnType<typeof selfResponse>>()
    getSpy.mockReturnValueOnce(oldRequest.promise)
    renderHarness()
    await flush()
    getSpy.mockResolvedValue(selfResponse({ id: 1, used_quota: 55 }))
    act(() => {
      seedSession('sid-2')
    })
    await flush()
    expect(selfCalls()).toBe(2)
    expect(useAuthStore.getState().auth.user?.used_quota).toBe(55)
    await act(async () => {
      oldRequest.resolve(selfResponse({ id: 1, used_quota: 999 }))
    })
    expect(useAuthStore.getState().auth.user?.used_quota).toBe(55)
  })

  function selfCalls() {
    return getSpy.mock.calls.filter(
      (call: unknown[]) => call[0] === '/api/user/self'
    ).length
  }
})
