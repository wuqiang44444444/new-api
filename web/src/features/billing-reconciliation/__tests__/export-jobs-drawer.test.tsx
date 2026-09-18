import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  fireEvent,
  render,
  screen,
  waitFor,
  cleanup,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import type { CustomerExportJobView } from '../export-api'
import { ExportJobsDrawer } from '../export-jobs-drawer'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn(), post: vi.fn() } }))

const clients: QueryClient[] = []
afterEach(async () => {
  cleanup()
  for (const client of clients.splice(0)) {
    await client.cancelQueries()
    client.clear()
  }
  vi.resetAllMocks()
})

function jobView(
  overrides: Partial<CustomerExportJobView>
): CustomerExportJobView {
  return {
    job_id: 'cex_a',
    user_id: 7,
    target_user_id: 7,
    job_type: 'statement_details',
    status: 'succeeded',
    filters: {
      field_version: 1,
      start_timestamp: 1788192000,
      end_timestamp: 1790784000,
      timezone: 'Asia/Shanghai',
    },
    progress: { scanned: 120, matched: 100, written: 100, files: 1 },
    cancel_requested: false,
    artifact: {
      files: [
        {
          object_key: 'exports/jobs/cex_a/export.csv',
          file_name: 'export.csv',
          size_bytes: 1024,
          line_count: 100,
          sha256: 'a'.repeat(64),
        },
      ],
      line_count: 100,
      size_bytes: 1024,
      generated_at: 1789000000,
      expires_at: 1789600000,
    },
    created_at: 1789000000,
    ...overrides,
  }
}

async function mountDrawer() {
  const i18n = createInstance().use(initReactI18next)
  await i18n.init({ lng: 'en', resources: { en: { translation: {} } } })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  clients.push(client)
  const view = render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18n}>
        <ExportJobsDrawer open onOpenChange={vi.fn()} />
      </I18nextProvider>
    </QueryClientProvider>
  )
  return { view, client, i18n }
}

it('lists jobs with their status and keeps succeeded jobs downloadable', async () => {
  vi.mocked(api.get).mockResolvedValueOnce({
    data: { success: true, message: '', data: { items: [jobView({})] } },
  })
  await mountDrawer()
  expect(await screen.findByText('Statement details')).toBeTruthy()
  expect(screen.getByText('Ready to download')).toBeTruthy()
  expect(screen.getByRole('button', { name: 'Download' })).toBeTruthy()
})

it('lets users cancel queued jobs but shows no download before completion', async () => {
  vi.mocked(api.get).mockResolvedValueOnce({
    data: {
      success: true,
      message: '',
      data: {
        items: [
          jobView({
            job_id: 'cex_b',
            status: 'queued',
            artifact: undefined,
            progress: { scanned: 0, matched: 0, written: 0, files: 0 },
          }),
        ],
      },
    },
  })
  vi.mocked(api.post).mockResolvedValueOnce({
    data: { success: true, message: '', data: {} },
  })
  await mountDrawer()
  expect(await screen.findByText('Queued')).toBeTruthy()
  expect(screen.queryByRole('button', { name: 'Download' })).toBeNull()
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
  await waitFor(() =>
    expect(api.post).toHaveBeenCalledWith('/api/billing/exports/cex_b/cancel')
  )
})

it('surfaces failed jobs without exposing internal error details', async () => {
  vi.mocked(api.get).mockResolvedValueOnce({
    data: {
      success: true,
      message: '',
      data: {
        items: [
          jobView({ job_id: 'cex_c', status: 'failed', artifact: undefined }),
        ],
      },
    },
  })
  await mountDrawer()
  expect(await screen.findByText('Failed')).toBeTruthy()
  expect(
    screen.getByText('Export failed. You can submit it again later.')
  ).toBeTruthy()
  expect(screen.queryByText(/storage_unavailable/)).toBeNull()
})

it('offers separate direct links for every file and refreshes expired links', async () => {
  const popup = vi.spyOn(window, 'open')
  const files = ['export.csv', 'export.part2.csv'].map((file_name) => ({
    file_name,
    url: `https://downloads.example/${file_name}`,
    expires_at: Math.floor(Date.now() / 1000) + 600,
    size_bytes: 2048,
    line_count: 100,
    sha256: 'a'.repeat(64),
  }))
  vi.mocked(api.get)
    .mockResolvedValueOnce({
      data: { success: true, data: { items: [jobView({})] } },
    })
    .mockResolvedValueOnce({ data: { success: true, data: { files } } })
    .mockResolvedValueOnce({
      data: {
        success: true,
        data: {
          files: files.map((f) => ({ ...f, url: `${f.url}?refreshed` })),
        },
      },
    })
  await mountDrawer()
  fireEvent.click(await screen.findByRole('button', { name: 'Download' }))
  for (const file of files) {
    const link = await screen.findByRole('link', {
      name: `Download ${file.file_name}`,
    })
    expect(link.getAttribute('href')).toBe(file.url)
    expect(link.getAttribute('rel')).toContain('noopener')
  }
  expect(popup).not.toHaveBeenCalled()
  fireEvent.click(
    screen.getByRole('button', { name: 'Refresh download links' })
  )
  await waitFor(() =>
    expect(
      screen
        .getByRole('link', { name: 'Download export.csv' })
        .getAttribute('href')
    ).toContain('?refreshed')
  )
  popup.mockRestore()
})

it('identifies the frozen customer and scope including zero IDs with keyboard access', async () => {
  const job = jobView({ target_user_id: 42 })
  job.filters = {
    ...job.filters,
    token_id: 0,
    channel_id: 9,
    token_name: 'frozen-key',
    group: 'vip',
    model_name: 'frozen-model',
    billing_mode: 'per_second',
    request_id: 'req-frozen',
    upstream_request_id: 'upstream-frozen',
    log_types: [2, 5],
  }
  vi.mocked(api.get).mockResolvedValueOnce({
    data: { success: true, data: { items: [job] } },
  })
  await mountDrawer()
  expect(await screen.findByText('Customer #42')).toBeTruthy()
  const trigger = screen.getByRole('button', { name: 'Export scope' })
  trigger.focus()
  await userEvent.keyboard('{Enter}')
  expect(await screen.findByText('API Key ID: 0')).toBeTruthy()
  expect(screen.getByText('frozen-key')).toBeTruthy()
  expect(screen.getByText('req-frozen')).toBeTruthy()
  expect(screen.getByText('upstream-frozen')).toBeTruthy()
})

it('shows cancellation immediately when requested and prevents duplicate cancellation', async () => {
  vi.mocked(api.get).mockResolvedValueOnce({
    data: {
      success: true,
      data: {
        items: [
          jobView({
            status: 'running',
            cancel_requested: true,
            artifact: undefined,
          }),
        ],
      },
    },
  })
  await mountDrawer()
  const button = await screen.findByRole('button', { name: 'Cancelling' })
  expect(button.hasAttribute('disabled')).toBe(true)
  expect(screen.queryByText('Generating')).toBeNull()
})

it('shows resource waiting while a job remains active', async () => {
  vi.mocked(api.get).mockResolvedValueOnce({
    data: {
      success: true,
      data: {
        items: [
          jobView({
            status: 'running',
            progress: {
              scanned: 10,
              matched: 5,
              written: 5,
              files: 1,
              waiting_for_resources: true,
            },
          }),
        ],
      },
    },
  })
  await mountDrawer()
  expect(await screen.findByText('Waiting for resources')).toBeTruthy()
  expect(screen.getByText('Scanned 10 records · 5 matched')).toBeTruthy()
})

it('does not offer expired signed URLs and lets users renew them', async () => {
  const file = {
    file_name: 'export.csv',
    url: 'https://downloads.example/old',
    expires_at: Math.floor(Date.now() / 1000) - 1,
    size_bytes: 100,
    line_count: 1,
    sha256: 'a'.repeat(64),
  }
  vi.mocked(api.get)
    .mockResolvedValueOnce({
      data: { success: true, data: { items: [jobView({})] } },
    })
    .mockResolvedValueOnce({ data: { success: true, data: { files: [file] } } })
    .mockResolvedValueOnce({
      data: {
        success: true,
        data: {
          files: [
            {
              ...file,
              url: 'https://downloads.example/new',
              expires_at: Math.floor(Date.now() / 1000) + 600,
            },
          ],
        },
      },
    })
  await mountDrawer()
  fireEvent.click(await screen.findByRole('button', { name: 'Download' }))
  expect(
    await screen.findByText('Download links expired. Refresh them to continue.')
  ).toBeTruthy()
  expect(screen.queryByRole('link')).toBeNull()
  fireEvent.click(
    screen.getByRole('button', { name: 'Refresh download links' })
  )
  expect(
    (
      await screen.findByRole('link', { name: 'Download export.csv' })
    ).getAttribute('href')
  ).toBe('https://downloads.example/new')
})

it('distinguishes loading and errors from an empty export history', async () => {
  let reject: (error: Error) => void = () => {}
  vi.mocked(api.get).mockReturnValueOnce(
    new Promise((_resolve, rejectPromise) => {
      reject = rejectPromise
    })
  )
  await mountDrawer()
  expect(screen.getByText('Loading...')).toBeTruthy()
  expect(screen.queryByText('No export jobs yet.')).toBeNull()
  reject(new Error('private network error'))
  expect(await screen.findByText('Unable to load export jobs.')).toBeTruthy()
  expect(screen.queryByText('No export jobs yet.')).toBeNull()
  expect(screen.queryByText('private network error')).toBeNull()
})
