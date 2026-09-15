import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { CHANNEL_TYPE_SEEDANCE_LINK } from '@/features/channels/constants'

import * as api from '../../api'
import type { TaskLog } from '../../types'
import { TaskDetailsDialog } from '../dialogs/task-details-dialog'

const client = new QueryClient({
  defaultOptions: { queries: { retry: false } },
})
afterEach(() => client.clear())
const notice =
  'Video downloads are temporary. Download your video promptly; long-term storage is not provided.'
const task: TaskLog = {
  id: 1,
  user_id: 1,
  platform: String(CHANNEL_TYPE_SEEDANCE_LINK),
  task_id: 'task_fixture',
  action: 'text_to_video',
  channel_id: 1,
  group: 'default',
  quota: 100,
  submit_time: 1,
  status: 'SUCCESS',
}

test('successful Seedance results explain temporary availability without promising a fixed lifetime', async () => {
  vi.spyOn(api, 'getTaskArtifacts').mockResolvedValue({ artifacts: [] })
  const { rerender } = render(
    <QueryClientProvider client={client}>
      <TaskDetailsDialog
        log={task}
        isAdmin={false}
        isRoot={false}
        open
        onOpenChange={() => undefined}
      />
    </QueryClientProvider>
  )
  expect(screen.getByText(notice)).toBeVisible()
  expect(screen.queryByText(/24 hours/)).not.toBeInTheDocument()
  await screen.findByText('No video result was recorded for this task')
  rerender(
    <QueryClientProvider client={client}>
      <TaskDetailsDialog
        log={{ ...task, status: 'RECONCILIATION_REQUIRED' }}
        isAdmin={false}
        isRoot={false}
        open
        onOpenChange={() => undefined}
      />
    </QueryClientProvider>
  )
  expect(screen.queryByText(notice)).not.toBeInTheDocument()
  rerender(
    <QueryClientProvider client={client}>
      <TaskDetailsDialog
        log={{ ...task, platform: 'suno' }}
        isAdmin={false}
        isRoot={false}
        open
        onOpenChange={() => undefined}
      />
    </QueryClientProvider>
  )
  expect(screen.queryByText(notice)).not.toBeInTheDocument()
})
