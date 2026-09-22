import { beforeEach, expect, it, vi } from 'vitest'

import {
  resubmitExport,
  type CustomerExportJobView,
} from '@/features/billing-reconciliation/export-api'

const post = vi.hoisted(() => vi.fn())
vi.mock('@/lib/api', () => ({ api: { post } }))
beforeEach(() => {
  post.mockReset()
  post.mockResolvedValue({
    data: { success: true, data: { job_id: 'new-job' } },
  })
})
it.each([
  { target: 7, endpoint: '/api/usage/self/exports' },
  { target: 0, endpoint: '/api/usage/admin/exports' },
  { target: 8, endpoint: '/api/usage/admin/exports' },
])(
  'regenerates usage scope through $endpoint preserving the source snapshot',
  async ({ target, endpoint }) => {
    const job = {
      job_id: 'source-job',
      job_type: 'usage_summary',
      user_id: 7,
      target_user_id: target,
      filters: {},
    } as CustomerExportJobView
    await resubmitExport(job)
    expect(post).toHaveBeenCalledExactlyOnceWith(
      endpoint,
      {
        source_job_id: 'source-job',
      },
      { skipErrorHandler: true }
    )
  }
)
