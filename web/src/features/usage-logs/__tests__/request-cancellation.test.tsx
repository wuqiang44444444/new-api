import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { AxiosError, CanceledError } from 'axios'
import { toast } from 'sonner'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { TaskRequestDetails } from '../components/task-request-details'

const originalAdapter = api.defaults.adapter
let client: QueryClient
let observer: number | undefined

beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  useAuthStore
    .getState()
    .auth.setUser({ id: 7, username: 'root', role: ROLE.SUPER_ADMIN })
  vi.spyOn(toast, 'error')
})

afterEach(() => {
  cleanup()
  client.clear()
  if (observer !== undefined) api.interceptors.response.eject(observer)
  observer = undefined
  api.defaults.adapter = originalAdapter
  useAuthStore.getState().auth.reset()
  vi.restoreAllMocks()
})

function openRequestDetails() {
  render(
    <QueryClientProvider client={client}>
      <TaskRequestDetails taskId='task-1' />
    </QueryClientProvider>
  )
  fireEvent.click(screen.getByRole('button', { name: 'User request details' }))
}

describe.each(['list', 'detail'] as const)(
  'Request shortcut HTTP handling during %s fetch',
  (phase) => {
    const pendingURL =
      phase === 'list'
        ? '/api/task_request_evidence'
        : '/api/task_request_evidence/1'

    it('does not toast when closing cancels the request through the real interceptor', async () => {
      let notifyStarted: (signal: AbortSignal) => void
      let notifyIntercepted: (error: unknown) => void
      const started = new Promise<AbortSignal>((resolve) => {
        notifyStarted = resolve
      })
      const intercepted = new Promise<unknown>((resolve) => {
        notifyIntercepted = resolve
      })
      const requested: string[] = []
      // Observe rejection after the production error interceptor has run.
      observer = api.interceptors.response.use(undefined, (error) => {
        notifyIntercepted(error)
        throw error
      })
      api.defaults.adapter = async (config) => {
        requested.push(config.url ?? '')
        if (config.url === pendingURL) {
          expect(config.signal).toBeInstanceOf(AbortSignal)
          const signal = config.signal as AbortSignal
          return new Promise((_, reject) => {
            signal.addEventListener('abort', () => {
              reject(new CanceledError('canceled', config))
            })
            notifyStarted(signal)
          })
        }
        return {
          config,
          status: 200,
          statusText: 'OK',
          headers: {},
          data: {
            success: true,
            data: { items: [{ id: 1, request_id: 'request-1' }], total: 1 },
          },
        }
      }

      openRequestDetails()
      const signal = await started
      await act(async () => {
        fireEvent.click(screen.getByRole('button', { name: 'Close' }))
        expect(await intercepted).toBeInstanceOf(CanceledError)
      })

      expect(signal.aborted).toBe(true)
      expect(screen.queryByRole('dialog')).toBeNull()
      expect(toast.error).not.toHaveBeenCalled()
      expect(requested).toEqual(
        phase === 'list'
          ? ['/api/task_request_evidence']
          : ['/api/task_request_evidence', '/api/task_request_evidence/1']
      )
      expect(client.getQueryCache().getAll()).toHaveLength(0)
    })

    it('shows a real network failure and lets the user retry successfully', async () => {
      let failed = false
      api.defaults.adapter = async (config) => {
        if (config.url === pendingURL && !failed) {
          failed = true
          throw new AxiosError('offline', AxiosError.ERR_NETWORK, config)
        }
        return {
          config,
          status: 200,
          statusText: 'OK',
          headers: {},
          data: {
            success: true,
            data: {
              items: failed ? [] : [{ id: 1, request_id: 'request-1' }],
              total: failed ? 0 : 1,
            },
          },
        }
      }

      openRequestDetails()
      expect(await screen.findByRole('alert')).toHaveTextContent(
        'Failed to load request evidence'
      )
      fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
      expect(
        await screen.findByText('No request evidence recorded')
      ).toBeVisible()
      expect(screen.queryByRole('alert')).toBeNull()
    })
  }
)
