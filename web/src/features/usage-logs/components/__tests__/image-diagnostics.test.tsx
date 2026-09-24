import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import type { TaskLog } from '../../types'
import { TaskDetailsDialog } from '../dialogs/task-details-dialog'

vi.mock('../task-evidence', () => ({ TaskEvidence: () => null }))
vi.mock('../task-request-details', () => ({ TaskRequestDetails: () => null }))

test.each([
  { isAdmin: false, isRoot: false },
  { isAdmin: true, isRoot: false },
  { isAdmin: true, isRoot: true },
])('image diagnostics respect role $isAdmin/$isRoot', async (role) => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        quota: 0,
        state: 'unknown',
        source: 'initial',
        evidence: 'historical',
        initial_evidence: 'historical',
      },
    },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const task: TaskLog = {
    id: 1,
    user_id: 1,
    task_id: 'task_image',
    platform: '1',
    channel_id: 1,
    group: 'default',
    quota: 0,
    submit_time: 1,
    action: 'image_generation',
    status: 'FAILURE',
    admin_info: {
      image_execution: { upstream_status: 422, violation_marker: false },
    },
    root_info: { upstream_request_id: 'provider-correlation' },
  }
  render(
    <QueryClientProvider client={client}>
      <TaskDetailsDialog
        log={task}
        {...role}
        open
        onOpenChange={() => undefined}
      />
    </QueryClientProvider>
  )
  expect(
    await screen.findByText(
      'Billing details were not fully recorded. The calculation cannot be reconstructed.'
    )
  ).toBeVisible()
  if (role.isAdmin) {
    expect(screen.getByText('Upstream HTTP status')).toBeVisible()
    expect(screen.getByText('422')).toBeVisible()
    expect(screen.getByText('Violation policy matched')).toBeVisible()
    expect(screen.getByText('No')).toBeVisible()
  } else {
    expect(screen.queryByText('Upstream HTTP status')).not.toBeInTheDocument()
    expect(
      screen.queryByText('Violation policy matched')
    ).not.toBeInTheDocument()
  }
  if (role.isRoot) {
    expect(screen.getByText('provider-correlation')).toBeVisible()
  } else {
    expect(screen.queryByText('provider-correlation')).not.toBeInTheDocument()
  }
})
