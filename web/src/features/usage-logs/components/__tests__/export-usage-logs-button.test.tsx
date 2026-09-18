import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { ExportUsageLogsButton } from '../export-usage-logs-button'

const state = vi.hoisted(() => ({
  search: {} as Record<string, unknown>,
  admin: false,
  selfExport: vi.fn().mockResolvedValue({}),
  adminExport: vi.fn().mockResolvedValue({}),
  error: vi.fn(),
}))
vi.mock('@tanstack/react-router', () => ({
  getRouteApi: () => ({ useSearch: () => state.search }),
}))
vi.mock('../usage-logs-provider', () => ({
  useLogsViewScope: () => ({ isAdminView: state.admin }),
}))
vi.mock('@/features/billing-reconciliation/export-api', () => ({
  createSelfExport: state.selfExport,
  createAdminExport: state.adminExport,
}))
vi.mock('@/features/billing-reconciliation/export-jobs-drawer', () => ({
  ExportJobsDrawer: () => null,
}))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: state.error } }))

beforeEach(() => {
  vi.clearAllMocks()
  state.admin = false
  state.search = { startTime: 1000000, endTime: 2000000 }
})
afterEach(cleanup)

it('keeps the customer, billing scope, playground key and inclusive final second', async () => {
  state.admin = true
  state.search = {
    ...state.search,
    billing: true,
    billingUserId: 27,
    billingMode: 'per_second',
    tokenId: 0,
    model: 'video',
    channel: '3',
    token: 'key',
    group: 'vip',
    requestId: 'req',
    upstreamRequestId: 'up',
  }
  render(<ExportUsageLogsButton />)
  fireEvent.click(screen.getByRole('button', { name: 'Export usage records' }))
  await waitFor(() =>
    expect(state.adminExport).toHaveBeenCalledWith(
      27,
      expect.objectContaining({
        job_type: 'statement_details',
        start_timestamp: 1000,
        end_timestamp: 2001,
        token_id: 0,
        channel_id: 3,
        billing_mode: 'per_second',
        model_name: 'video',
        token_name: 'key',
        group: 'vip',
        request_id: 'req',
        upstream_request_id: 'up',
      })
    )
  )
  expect(state.selfExport).not.toHaveBeenCalled()
})

it('requires an explicit customer when viewing all users', () => {
  state.admin = true
  render(<ExportUsageLogsButton />)
  fireEvent.click(screen.getByRole('button', { name: 'Export usage records' }))
  expect(state.error).toHaveBeenCalledWith('Select customer')
  expect(state.selfExport).not.toHaveBeenCalled()
  expect(state.adminExport).not.toHaveBeenCalled()
})

it('submits the explicit administrator username to resolve a stable customer ID', async () => {
  state.admin = true
  state.search = {
    ...state.search,
    username: 'customer',
    model: 'model%',
    type: ['6'],
  }
  render(<ExportUsageLogsButton />)
  fireEvent.click(screen.getByRole('button', { name: 'Export usage records' }))
  await waitFor(() =>
    expect(state.adminExport).toHaveBeenCalledWith(
      'customer',
      expect.objectContaining({
        username: 'customer',
        model_name: 'model%',
        log_types: [6],
      })
    )
  )
})

it('uses self scope after an administrator switches to only mine', async () => {
  state.search = {
    ...state.search,
    username: 'another',
    channel: '3',
    group: 'vip',
    token: 'key',
    requestId: 'req',
  }
  render(<ExportUsageLogsButton />)
  fireEvent.click(screen.getByRole('button', { name: 'Export usage records' }))
  await waitFor(() =>
    expect(state.selfExport).toHaveBeenCalledWith(
      expect.objectContaining({
        group: 'vip',
        token_name: 'key',
        request_id: 'req',
        username: undefined,
        channel_id: undefined,
      })
    )
  )
  expect(state.adminExport).not.toHaveBeenCalled()
})
