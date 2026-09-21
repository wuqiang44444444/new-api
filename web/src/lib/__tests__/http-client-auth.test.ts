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
import axios from 'axios'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  AuthRefreshSupersededError,
  isAuthRefreshSuperseded,
} from '@/lib/auth-session'
import { useAuthStore } from '@/stores/auth-store'
import { api } from '../http-client'

const refreshAuthentication = vi.hoisted(() => vi.fn())

vi.mock('@/lib/auth-session', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/auth-session')>()
  return {
    ...actual,
    refreshAuthentication: (...args: unknown[]) =>
      refreshAuthentication(...args),
  }
})

type QueueStep = { status: number; token?: string }

function readAuthorization(headers: unknown): string {
  const view = headers as {
    get?: (key: string) => string | undefined
    Authorization?: string
  }
  return String(view?.get?.('Authorization') ?? view?.Authorization ?? '')
}

function queueAdapter(steps: QueueStep[]) {
  const seen: string[] = []
  api.defaults.adapter = async (config) => {
    seen.push(readAuthorization(config.headers))
    const step = steps[Math.min(seen.length - 1, steps.length - 1)]
    const response = {
      data: { success: true, data: {} },
      status: step.status,
      statusText: 'OK',
      headers: {} as Record<string, string>,
      config,
    }
    // Mirror the core adapters: a non-2xx status must reject so the
    // response error interceptor (the 401 recovery path) runs.
    if (!config.validateStatus?.(step.status)) {
      throw new axios.AxiosError(
        'Request failed with status error',
        axios.AxiosError.ERR_BAD_REQUEST,
        config,
        null,
        response as never
      )
    }
    return response
  }
  return seen
}

function applyStoreToken(token: string) {
  useAuthStore.setState((state) => ({
    auth: { ...state.auth, accessToken: token },
  }))
}

describe('http-client 401 recovery', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useAuthStore.setState((state) => ({
      auth: { ...state.auth, accessToken: 'stale-token' },
    }))
    refreshAuthentication.mockReset()
  })

  it('retries once when the refresh outcome is authenticated', async () => {
    refreshAuthentication.mockImplementation(async () => {
      applyStoreToken('fresh-token')
      return { kind: 'authenticated' }
    })
    const seen = queueAdapter([{ status: 401 }, { status: 200 }])

    const response = await api.get('/api/probe')

    expect(response.status).toBe(200)
    expect(seen).toEqual(['Bearer stale-token', 'Bearer fresh-token'])
    expect(refreshAuthentication).toHaveBeenCalledTimes(1)
  })

  it('retries once when a concurrent refresh superseded this one', async () => {
    refreshAuthentication.mockImplementation(async () => {
      // The winning refresh installed its bundle before this attempt
      // observed the epoch change and bailed out with a superseded error.
      applyStoreToken('fresh-token')
      return { kind: 'transient_error', error: new AuthRefreshSupersededError() }
    })
    const seen = queueAdapter([{ status: 401 }, { status: 200 }])

    const response = await api.get('/api/probe')

    expect(response.status).toBe(200)
    expect(seen).toEqual(['Bearer stale-token', 'Bearer fresh-token'])
    expect(refreshAuthentication).toHaveBeenCalledTimes(1)
  })

  it('does not retry a plain transient refresh failure', async () => {
    refreshAuthentication.mockResolvedValue({
      kind: 'transient_error',
      error: new Error('refresh network failed'),
    })
    const seen = queueAdapter([{ status: 401 }])

    await expect(api.get('/api/probe')).rejects.toMatchObject({
      response: { status: 401 },
    })
    expect(seen).toEqual(['Bearer stale-token'])
  })

  it('recognises superseded errors without matching other failures', () => {
    expect(isAuthRefreshSuperseded(new AuthRefreshSupersededError())).toBe(
      true
    )
    expect(isAuthRefreshSuperseded(new Error('boom'))).toBe(false)
    expect(isAuthRefreshSuperseded(null)).toBe(false)
  })
})
