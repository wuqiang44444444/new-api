import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'

import { ExportJobsDrawer } from '../../export-jobs-drawer'

const list = vi.hoisted(() => vi.fn())
vi.mock('../../export-api', async (original) => ({
  ...(await original<typeof import('../../export-api')>()),
  listSelfExports: list,
}))
vi.mock('react-i18next', async (original) => ({
  ...(await original<typeof import('react-i18next')>()),
  useTranslation: () => ({ t: (key: string) => key }),
}))

it('loads older export jobs with server pagination and does not append stale jobs', async () => {
  list.mockImplementation(async (page: number, pageSize: number) => ({
    page,
    page_size: pageSize,
    total: 21,
    items: [
      {
        job_id: `job-${page}`,
        user_id: 1,
        target_user_id: 1,
        job_type: 'usage_logs',
        status: 'cancelled',
        filters: {
          field_version: 1,
          start_timestamp: 1000,
          end_timestamp: 2000,
          timezone: 'Asia/Shanghai',
          model_name: `page-${page}-model`,
        },
        progress: { scanned: 0, matched: 0, written: 0, files: 0 },
        cancel_requested: false,
        created_at: 1000,
      },
    ],
  }))
  const user = userEvent.setup()
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <ExportJobsDrawer open onOpenChange={vi.fn()} />
    </QueryClientProvider>
  )
  expect(await screen.findByText('Model: page-1-model')).toBeVisible()
  await user.click(
    within(screen.getByRole('group', { name: 'Export job pages' })).getByRole(
      'button',
      { name: 'Go to next page' }
    )
  )
  expect(await screen.findByText('Model: page-2-model')).toBeVisible()
  expect(list).toHaveBeenLastCalledWith(2, 20)
  expect(screen.queryByText('Model: page-1-model')).not.toBeInTheDocument()
})
