import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, expect, it, vi } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'

import { StatementVersionPanel } from '../statement-version-panel'

const api = vi.hoisted(() => ({
  status: vi.fn(),
  confirm: vi.fn(),
  diff: vi.fn(),
  cleanup: vi.fn(),
  download: vi.fn(),
  verify: vi.fn(),
  source: vi.fn(),
}))
vi.mock('../../source-review-api', () => ({
  getSourceReview: api.source,
  recordSourceReview: vi.fn(),
}))
vi.mock('../../version-api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../version-api')>()),
  getAdminVersionMonthStatus: api.status,
  confirmAdminVersion: api.confirm,
  getAdminVersionDiff: api.diff,
  cleanupAdminVersion: api.cleanup,
  getAdminVersionDownload: api.download,
  verifyAdminStatementSource: api.verify,
}))

it('requires acknowledgment of the pending draft and preserves its correction reason', async () => {
  const draft = {
    id: 2,
    draft_public_id: 'draft-two',
    status: 'pending',
    corrects_version_id: 1,
    public_reason: 'Verified adjustment',
    quota_per_unit: 1000000,
    currency: 'USD',
    currency_rate: 1,
    data_quality: {
      status: 'partial',
      unavailable_requests: 1,
      unknown_billing_mode_requests: 5,
    },
  }
  api.status.mockResolvedValue({
    success: true,
    data: {
      switch_enabled: true,
      topology_ok: true,
      retention_status: 'intact',
      current_version: null,
      active_draft: draft,
      has_active_draft: true,
      versions: [],
    },
  })
  api.diff.mockResolvedValue({
    success: true,
    data: { diff: { items: {}, groups: [], models: [], quality: {} } },
  })
  api.confirm.mockResolvedValue({ success: true, committed: true })
  const i18n = createInstance().use(initReactI18next)
  await i18n.init({ lng: 'en', resources: { en: { translation: {} } } })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const view = render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <StatementVersionPanel
          isAdmin
          userId={11}
          period={{ start_timestamp: 100, end_timestamp: 200 }}
          previewVersionId={null}
          onPreviewChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </QueryClientProvider>
    </I18nextProvider>
  )
  fireEvent.click(
    await screen.findByRole('button', { name: 'Confirm statement' })
  )
  const dialog = await screen.findByRole('dialog')
  expect(
    within(dialog).getByText('Unknown billing mode: 5 records')
  ).toBeInTheDocument()
  const confirm = within(dialog).getByRole('button', {
    name: 'Confirm statement',
  })
  expect((confirm as HTMLButtonElement).disabled).toBe(true)
  expect((within(dialog).getByRole('textbox') as HTMLInputElement).value).toBe(
    'Verified adjustment'
  )
  fireEvent.click(within(dialog).getByRole('checkbox'))
  await waitFor(() =>
    expect((confirm as HTMLButtonElement).disabled).toBe(false)
  )
  fireEvent.click(confirm)
  await waitFor(() => expect(api.confirm).toHaveBeenCalled())
  const [id, body] = api.confirm.mock.calls[0]
  expect(id).toBe('draft-two')
  expect(body.base_version_id).toBe(1)
  expect(body.public_reason).toBe('Verified adjustment')
  expect(JSON.parse(body.acknowledged_quality)).toMatchObject({
    acknowledged: true,
    draft_public_id: 'draft-two',
  })
  view.unmount()
  client.clear()
})

afterEach(() => {
  cleanup()
  useAuthStore.getState().auth.reset()
  vi.useRealTimers()
  vi.clearAllMocks()
})

it('lets root register source evidence before enabling generation', async () => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
  const status = {
    switch_enabled: true,
    topology_ok: true,
    retention_status: 'unknown',
    current_version: null,
    active_draft: null,
    has_active_draft: false,
    versions: [],
  }
  api.status.mockResolvedValue({ success: true, data: status })
  api.verify.mockResolvedValue({ success: true })
  const close = await renderVersionPanel()
  const generate = await screen.findByRole('button', {
    name: 'Generate pending version',
  })
  expect(generate).toBeDisabled()
  fireEvent.click(
    screen.getByRole('button', { name: 'Review and differences' })
  )
  const dialog = await screen.findByRole('dialog')
  fireEvent.click(
    await within(dialog).findByRole('button', {
      name: 'Technical source verification',
    })
  )
  const submit = within(dialog).getByRole('button', {
    name: 'Record verified completeness',
  })
  expect(submit).toBeDisabled()
  fireEvent.change(
    await within(dialog).findByRole('textbox', {
      name: 'Backup and coverage evidence',
    }),
    { target: { value: '   ' } }
  )
  expect(submit).toBeDisabled()
  fireEvent.change(
    await within(dialog).findByRole('textbox', {
      name: 'Backup and coverage evidence',
    }),
    { target: { value: ' Verified backup and reconciliation record AUDIT-8 ' } }
  )
  fireEvent.change(
    await within(dialog).findByRole('textbox', {
      name: 'Log retention and recovery evidence',
    }),
    { target: { value: 'Retention checked' } }
  )
  api.status.mockResolvedValue({
    success: true,
    data: { ...status, retention_status: 'intact' },
  })
  fireEvent.click(submit)
  await waitFor(() =>
    expect(api.verify).toHaveBeenCalledWith(
      { user_id: 11, start_timestamp: 100, end_timestamp: 200 },
      {
        fingerprint: 'source-fixture',
        backup_evidence: 'Verified backup and reconciliation record AUDIT-8',
        retention_evidence: 'Retention checked',
      }
    )
  )
  await waitFor(() => expect(generate).toBeEnabled())
  close()
})

it('keeps source verification unavailable to non-root administrators', async () => {
  useAuthStore.getState().auth.setUser({ id: 2, username: 'admin', role: 10 })
  api.status.mockResolvedValue({
    success: true,
    data: {
      switch_enabled: true,
      topology_ok: true,
      retention_status: 'unknown',
      current_version: null,
      active_draft: null,
      has_active_draft: false,
      versions: [],
    },
  })
  const close = await renderVersionPanel()
  expect(
    await screen.findByRole('button', { name: 'Generate pending version' })
  ).toBeDisabled()
  fireEvent.click(
    screen.getByRole('button', { name: 'Review and differences' })
  )
  await screen.findByText(
    'A super administrator must verify statement sources first.'
  )
  expect(
    screen.queryByRole('button', { name: 'Record verified completeness' })
  ).toBeNull()
  expect(
    screen.getByText(
      'A super administrator must verify statement sources first.'
    )
  ).toBeInTheDocument()
  expect(api.verify).not.toHaveBeenCalled()
  close()
})

it('preserves evidence and keeps generation blocked when verification fails', async () => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
  api.status.mockResolvedValue({
    success: true,
    data: {
      switch_enabled: true,
      topology_ok: true,
      retention_status: 'unknown',
      current_version: null,
      active_draft: null,
      has_active_draft: false,
      versions: [],
    },
  })
  api.verify.mockResolvedValue({
    success: false,
    message: 'Source verification rejected',
  })
  const close = await renderVersionPanel()
  fireEvent.click(
    await screen.findByRole('button', { name: 'Review and differences' })
  )
  const dialog = await screen.findByRole('dialog')
  fireEvent.click(
    await within(dialog).findByRole('button', {
      name: 'Technical source verification',
    })
  )
  const input = await within(dialog).findByRole('textbox', {
    name: 'Backup and coverage evidence',
  })
  fireEvent.change(input, { target: { value: 'AUDIT-8' } })
  fireEvent.change(
    within(dialog).getByRole('textbox', {
      name: 'Log retention and recovery evidence',
    }),
    { target: { value: 'Retention checked' } }
  )
  const submit = within(dialog).getByRole('button', {
    name: 'Record verified completeness',
  })
  fireEvent.click(submit)
  await waitFor(() => expect(api.verify).toHaveBeenCalledTimes(1))
  await waitFor(() => expect(submit).toBeEnabled())
  expect(input).toHaveValue('AUDIT-8')
  expect(
    screen.getByRole('button', {
      name: 'Generate pending version',
      hidden: true,
    })
  ).toBeDisabled()
  close()
})

it('explains unsupported database topology and does not allow source verification', async () => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
  api.status.mockResolvedValue({
    success: true,
    data: {
      switch_enabled: true,
      topology_ok: false,
      retention_status: 'unknown',
      current_version: null,
      active_draft: null,
      has_active_draft: false,
      versions: [],
    },
  })
  const close = await renderVersionPanel()
  expect(
    await screen.findByRole('button', { name: 'Generate pending version' })
  ).toBeDisabled()
  expect(
    screen.getByRole('button', { name: 'Review and differences' })
  ).toBeDisabled()
  expect(screen.getByRole('alert')).toHaveTextContent(
    'Statement generation requires billing records and account data in the same database.'
  )
  close()
})

async function renderVersionPanel() {
  api.source.mockResolvedValue({
    success: true,
    data: {
      fingerprint: 'source-fixture',
      decision_version: 3,
      rows: 0,
      issues: [],
      blockers: 0,
      pending: 0,
      captured_at: 100,
    },
  })

  const i18n = createInstance().use(initReactI18next)
  await i18n.init({ lng: 'en', resources: { en: { translation: {} } } })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  let view: ReturnType<typeof render>
  await act(async () => {
    view = render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={client}>
          <StatementVersionPanel
            isAdmin
            userId={11}
            period={{ start_timestamp: 100, end_timestamp: 200 }}
            previewVersionId={null}
            onPreviewChange={vi.fn()}
            onChanged={vi.fn()}
          />
        </QueryClientProvider>
      </I18nextProvider>
    )
  })
  return () => {
    view.unmount()
    client.clear()
  }
}

it('refreshes a queued draft through generation to confirmation and stops polling', async () => {
  const draft = {
    id: 2,
    draft_public_id: 'queued-draft',
    status: 'queued',
    data_quality: { status: 'complete' },
  }
  const response = (status: string) => ({
    success: true,
    data: {
      switch_enabled: true,
      topology_ok: true,
      retention_status: 'intact',
      current_version: null,
      active_draft: { ...draft, status },
      has_active_draft: true,
      versions: [],
    },
  })
  api.status
    .mockReset()
    .mockResolvedValueOnce(response('queued'))
    .mockResolvedValueOnce(response('generating'))
    .mockResolvedValue(response('pending'))
  vi.useFakeTimers()
  const dispose = await renderVersionPanel()
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1)
  })
  expect(screen.queryByRole('button', { name: 'Confirm statement' })).toBeNull()
  await act(async () => {
    await vi.advanceTimersByTimeAsync(5000)
  })
  await act(async () => {
    await vi.advanceTimersByTimeAsync(5000)
  })
  expect(
    screen.getByRole('button', { name: 'Preview pending version' })
  ).toBeEnabled()
  expect(
    screen.getByRole('button', { name: 'Confirm statement' })
  ).toBeEnabled()
  const requestsAtCompletion = api.status.mock.calls.length
  await act(async () => {
    await vi.advanceTimersByTimeAsync(15000)
  })
  expect(api.status).toHaveBeenCalledTimes(requestsAtCompletion)
  dispose()
})

it.each(['failed', 'cancelled', 'invalid', 'cleaning'])(
  'cleans a %s historical draft without an active pointer, even with generation disabled',
  async (status) => {
    api.status.mockReset().mockResolvedValue({
      success: true,
      data: {
        switch_enabled: false,
        topology_ok: true,
        retention_status: 'intact',
        current_version: null,
        active_draft: null,
        has_active_draft: false,
        versions: [{ id: 2, draft_public_id: 'old-draft', status }],
      },
    })
    api.cleanup.mockResolvedValue({ success: true })
    const dispose = await renderVersionPanel()
    fireEvent.click(
      await screen.findByRole('button', { name: 'Version history (1)' })
    )
    fireEvent.click(
      await screen.findByRole('button', { name: 'Clean up draft' })
    )
    await waitFor(() => expect(api.cleanup).toHaveBeenCalledWith('old-draft'))
    dispose()
  }
)

it('downloads confirmation notes for the current and selected historical version', async () => {
  const current = {
    id: 2,
    draft_public_id: 'current-version',
    status: 'confirmed',
    version_number: 2,
  }
  const historical = {
    id: 1,
    draft_public_id: 'previous-version',
    status: 'confirmed',
    version_number: 1,
    public_reason: 'Previous confirmation',
  }
  api.status.mockReset().mockResolvedValue({
    success: true,
    data: {
      switch_enabled: true,
      topology_ok: true,
      retention_status: 'intact',
      current_version: current,
      active_draft: null,
      has_active_draft: false,
      versions: [current, historical],
    },
  })
  api.download.mockResolvedValue({
    success: true,
    data: { url: 'https://download.example/confirmation.txt' },
  })
  const open = vi.spyOn(window, 'open').mockReturnValue(null)
  const dispose = await renderVersionPanel()
  try {
    fireEvent.click(
      await screen.findByRole('button', { name: 'Download confirmation note' })
    )
    await waitFor(() =>
      expect(api.download).toHaveBeenCalledWith(
        'current-version',
        'confirmation_note'
      )
    )
    await waitFor(() =>
      expect(open).toHaveBeenCalledWith(
        'https://download.example/confirmation.txt',
        '_blank',
        'noopener'
      )
    )
    fireEvent.click(screen.getByRole('button', { name: 'Version history (2)' }))
    const row = await screen.findByRole('row', {
      name: /Previous confirmation/,
    })
    fireEvent.click(
      within(row).getByRole('button', { name: 'Download confirmation note' })
    )
    await waitFor(() =>
      expect(api.download).toHaveBeenCalledWith(
        'previous-version',
        'confirmation_note'
      )
    )
  } finally {
    open.mockRestore()
    dispose()
  }
})

it('keeps administrative source review available while generation is disabled', async () => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
  api.status.mockResolvedValue({
    success: true,
    data: {
      switch_enabled: false,
      topology_ok: true,
      retention_status: 'unknown',
      current_version: null,
      active_draft: null,
      has_active_draft: false,
      versions: [],
    },
  })
  const close = await renderVersionPanel()
  expect(
    await screen.findByRole('button', { name: 'Review and differences' })
  ).toBeEnabled()
  expect(
    screen.getByRole('button', { name: 'Generate pending version' })
  ).toBeDisabled()
  close()
})
