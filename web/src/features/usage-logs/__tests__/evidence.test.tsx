import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { TaskEvidence } from '../components/task-evidence'
import { getEvidenceList } from '../evidence-api'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn() } }))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
function show(isRoot = false) {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <TaskEvidence taskId='task-1' isRoot={isRoot} />
    </QueryClientProvider>
  )
}
describe('Task evidence', () => {
  it('loads only when opened and displays the empty state', async () => {
    vi.mocked(api.get).mockResolvedValue({
      data: { success: true, data: { items: [], total: 0 } },
    })
    show()
    expect(api.get).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Request evidence' }))
    expect(await screen.findByText('No request evidence recorded')).toBeTruthy()
  })
  it('shows an error and a retry action when the query fails', async () => {
    vi.mocked(api.get).mockRejectedValue(new Error('offline'))
    show()
    fireEvent.click(screen.getByRole('button', { name: 'Request evidence' }))
    expect(await screen.findByRole('alert')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeTruthy()
  })
  it('shows incomplete evidence and hides original downloads from administrators', async () => {
    vi.mocked(api.get)
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: { items: [{ id: 1, request_id: 'req-1' }], total: 1 },
        },
      })
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: {
            evidence: { body_expired: false },
            events: [
              {
                id: 2,
                stage: 'upstream_response',
                phase: 'failed',
                complete: false,
                has_body: true,
                preview: 'partial',
                byte_count: 7,
              },
            ],
          },
        },
      })
    show()
    fireEvent.click(screen.getByRole('button', { name: 'Request evidence' }))
    fireEvent.click(await screen.findByRole('button', { name: 'req-1' }))
    await waitFor(() => expect(screen.getByText(/Incomplete/)).toBeTruthy())
    expect(
      screen.queryByRole('button', { name: 'Download original evidence' })
    ).toBeNull()
  })
})

it('queries synchronous audio evidence by platform request ID without a task ID', async () => {
  vi.mocked(api.get).mockResolvedValue({
    data: { success: true, data: { items: [], total: 0 } },
  })
  await getEvidenceList({ request_id: 'audio-request' }, 1)
  expect(api.get).toHaveBeenCalledWith('/api/task_request_evidence', {
    params: { request_id: 'audio-request', p: 1, page_size: 20 },
  })
})

it.each([
  ['decrypt_failed', 'Evidence body cannot be decrypted or authenticated'],
  ['missing', 'Evidence body file is missing'],
  ['integrity_failed', 'Evidence body integrity check failed'],
  ['binary', 'Binary evidence has no text preview'],
])(
  'explains %s without rewriting capture completeness',
  async (status, message) => {
    vi.mocked(api.get)
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: { items: [{ id: 1, request_id: 'req-readability' }], total: 1 },
        },
      })
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: {
            evidence: { body_expired: false },
            events: [
              {
                id: 2,
                stage: 'upstream_response',
                phase: 'completed',
                complete: true,
                has_body: true,
                preview: '',
                body_status: status,
                byte_count: 8,
              },
            ],
          },
        },
      })
    show(true)
    fireEvent.click(screen.getByRole('button', { name: 'Request evidence' }))
    fireEvent.click(
      await screen.findByRole('button', { name: 'req-readability' })
    )
    expect(await screen.findByText(message)).toBeTruthy()
    expect(screen.getByText(/Complete/)).toBeTruthy()
    // Root can retry a download after temporary storage failure; permissions stay unchanged.
    expect(
      screen.getByRole('button', { name: 'Download original evidence' })
    ).toBeTruthy()
  }
)
