import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  render,
  screen,
  fireEvent,
  waitFor,
  cleanup,
  within,
} from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, expect, it, vi } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'

import { StatementSourceVerification } from '../statement-source-verification'

const api = vi.hoisted(() => ({
  get: vi.fn(),
  record: vi.fn(),
  verify: vi.fn(),
}))
vi.mock('../../source-review-api', () => ({
  getSourceReview: api.get,
  recordSourceReview: api.record,
}))
vi.mock('../../version-api', () => ({ verifyAdminStatementSource: api.verify }))
afterEach(() => {
  cleanup()
  useAuthStore.getState().auth.reset()
  vi.clearAllMocks()
})
const issue = {
  id: 'log:22816',
  decision_id: 0,
  kind: 'explanation',
  blocking: false,
  log_id: 22816,
  task_row_id: 0,
  created_at: 1785513700,
  model: 'seedance-2-0-oversea',
  log_type: 2,
  quota: '913500',
  related_log_ids: [22816],
  reasons: ['unknown_billing_mode'],
  reviewed: false,
  note: '',
  actor_id: 0,
  reviewed_at: 0,
}
async function open() {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
  const i18n = createInstance().use(initReactI18next)
  await i18n.init({ lng: 'en', resources: { en: { translation: {} } } })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <StatementSourceVerification
          userId={91}
          period={{ start_timestamp: 1785513600, end_timestamp: 1788191999 }}
          disabled={false}
          onVerified={vi.fn()}
        />
      </QueryClientProvider>
    </I18nextProvider>
  )
  fireEvent.click(
    screen.getByRole('button', { name: 'Review and differences' })
  )
  return client
}
it('keeps records without invented evidence, while amount differences remain pending', async () => {
  api.get.mockResolvedValue({
    success: true,
    data: {
      fingerprint: 'source',
      decision_version: 3,
      rows: 2,
      history: [],
      issues: [
        issue,
        {
          ...issue,
          id: 'task:159',
          kind: 'task_amount',
          blocking: true,
          quota: '29634',
          target_quota: '763138',
          reasons: ['task_log_net_mismatch'],
        },
      ],
      blockers: 1,
      pending: 2,
      captured_at: 100,
    },
  })
  api.record.mockResolvedValue({ success: true, decision_version: 3 })
  const client = await open()
  const consume = within(
    await screen.findByRole('article', { name: 'Billing item log:22816' })
  )
  expect(
    screen.queryByRole('textbox', { name: 'Backup and coverage evidence' })
  ).toBeNull()
  fireEvent.change(
    consume.getByRole('combobox', { name: 'Handling decision' }),
    { target: { value: 'keep_record' } }
  )
  expect(
    consume.getByRole('textbox', { name: 'Additional note (optional)' })
  ).toHaveValue('')
  expect(
    consume.getByText(
      'Statement adjustment: $0. Customer balance change: $0. No additional charge or refund.'
    )
  ).toBeInTheDocument()
  fireEvent.click(
    consume.getByRole('button', { name: 'Save handling decision' })
  )
  await waitFor(() =>
    expect(api.record).toHaveBeenCalledWith(
      expect.objectContaining({ user_id: 91 }),
      expect.objectContaining({
        fingerprint: 'source',
        issue_id: 'log:22816',
        decision: 'keep_record',
        reason: 'accept_missing_details',
        note: '',
        previous_id: 0,
        request_id: expect.any(String),
      })
    )
  )
  const task = within(
    screen.getByRole('article', { name: 'Billing item task:159' })
  )
  expect(
    task.getByRole('option', { name: 'Keep the record; no additional refund' })
  ).toBeDisabled()
  fireEvent.change(
    task.getByRole('textbox', { name: 'What needs to be reviewed?' }),
    { target: { value: 'Customer questions the amount' } }
  )
  fireEvent.click(task.getByRole('button', { name: 'Save handling decision' }))
  await waitFor(() =>
    expect(api.record).toHaveBeenLastCalledWith(
      expect.anything(),
      expect.objectContaining({ decision: 'investigate', issue_id: 'task:159' })
    )
  )
  fireEvent.click(
    screen.getByRole('button', { name: 'Technical source verification' })
  )
  expect(
    screen.getByRole('button', { name: 'Record verified completeness' })
  ).toBeDisabled()
  expect(api.verify).not.toHaveBeenCalled()
  client.clear()
})
it('records adjustment requests as pending, and retries the same request after a failed response', async () => {
  api.get.mockResolvedValue({
    success: true,
    data: {
      fingerprint: 'source',
      decision_version: 3,
      rows: 1,
      history: [],
      issues: [issue],
      blockers: 0,
      pending: 1,
      captured_at: 100,
    },
  })
  api.record
    .mockRejectedValueOnce(new Error('response lost'))
    .mockResolvedValue({ success: true, decision_version: 3 })
  const client = await open()
  const item = within(
    await screen.findByRole('article', { name: 'Billing item log:22816' })
  )
  fireEvent.change(item.getByRole('combobox', { name: 'Handling decision' }), {
    target: { value: 'request_waiver' },
  })
  expect(
    item.getByText(
      'This saves a pending request only. No fee is waived, no record is excluded, and no refund is executed. Review remains incomplete.'
    )
  ).toBeInTheDocument()
  fireEvent.click(item.getByRole('button', { name: 'Save handling decision' }))
  expect(await item.findByRole('alert')).toHaveTextContent(
    'Describe the reason'
  )
  expect(api.record).not.toHaveBeenCalled()
  fireEvent.change(
    item.getByRole('textbox', { name: 'What needs to be reviewed?' }),
    { target: { value: 'Proposed service compensation' } }
  )
  fireEvent.click(item.getByRole('button', { name: 'Save handling decision' }))
  await item.findByText(
    'Unable to save the decision. Refresh to check for changes before retrying; do not assume it was completed.'
  )
  fireEvent.click(item.getByRole('button', { name: 'Save handling decision' }))
  await waitFor(() => expect(api.record).toHaveBeenCalledTimes(2))
  expect(api.record.mock.calls[1][1]).toEqual(api.record.mock.calls[0][1])
  expect(api.record.mock.calls[0][1]).toMatchObject({
    decision: 'request_waiver',
    reason: 'customer_agreement',
  })
  client.clear()
})
it('shows historical decisions with actual effects without treating stale notes as verified', async () => {
  api.get.mockResolvedValue({
    success: true,
    data: {
      fingerprint: 'source',
      decision_version: 3,
      rows: 0,
      issues: [],
      blockers: 0,
      pending: 0,
      captured_at: 100,
      history: [
        {
          id: 1,
          version: 2,
          issue_id: 'log:22816',
          decision: 'request_duplicate_exclusion',
          reason: 'suspected_duplicate',
          actor_id: 7,
          created_at: 100,
          model: 'old-video',
          recorded_quota: '913500',
          log_type: 2,
          statement_delta: '0',
          balance_delta: '0',
          status: 'pending_review',
          note: 'Possible duplicate invoice line',
          current_source: false,
          previous_id: 0,
        },
      ],
    },
  })
  const client = await open()
  fireEvent.click(
    await screen.findByRole('button', { name: 'Handling history (1)' })
  )
  expect(
    screen.getByText('Request removal of a duplicate charge')
  ).toBeInTheDocument()
  expect(
    screen.getByText(
      'Request recorded; processing is pending. This is not a completed adjustment or refund.'
    )
  ).toBeInTheDocument()
  expect(
    screen.getByText(
      'Historical context only. This decision does not verify the current records.'
    )
  ).toBeInTheDocument()
  expect(
    screen.getByText('Possible duplicate invoice line')
  ).toBeInTheDocument()
  client.clear()
})
it('paginates all review items and treats scan failures as unknown, not complete', async () => {
  api.get.mockResolvedValueOnce({
    success: true,
    data: {
      fingerprint: 'scope',
      decision_version: 3,
      rows: 11,
      issues: Array.from({ length: 11 }, (_, i) => ({
        ...issue,
        id: `log:${i}`,
      })),
      blockers: 0,
      pending: 11,
      captured_at: 100,
    },
  })
  const client = await open()
  await screen.findByRole('article', { name: 'Billing item log:0' })
  fireEvent.click(screen.getByRole('button', { name: 'Next' }))
  expect(
    screen.getByRole('article', { name: 'Billing item log:10' })
  ).toBeInTheDocument()
  api.get.mockRejectedValue(new Error('scan unavailable'))
  fireEvent.click(screen.getByRole('button', { name: 'Run checks again' }))
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Source checks failed.'
  )
  expect(
    screen.queryByRole('button', { name: 'Record verified completeness' })
  ).toBeNull()
  expect(api.verify).not.toHaveBeenCalled()
  client.clear()
})

it('refuses an older backend response instead of sending new decisions to the old contract', async () => {
  api.get.mockResolvedValue({
    success: true,
    data: {
      fingerprint: 'old',
      rows: 1,
      issues: [issue],
      blockers: 0,
      pending: 1,
      captured_at: 100,
    },
  })
  const client = await open()
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Source checks failed.'
  )
  expect(
    screen.queryByRole('button', { name: 'Save handling decision' })
  ).toBeNull()
  expect(api.record).not.toHaveBeenCalled()
  client.clear()
})

it('preserves another item draft when a saved decision refreshes the report', async () => {
  const report = {
    fingerprint: 'source',
    decision_version: 3,
    rows: 2,
    issues: [issue, { ...issue, id: 'log:2' }],
    blockers: 0,
    pending: 2,
    captured_at: 100,
    history: [],
  }
  api.get.mockResolvedValueOnce({ success: true, data: report })
  api.record.mockResolvedValue({ success: true, decision_version: 3 })
  let finish!: (value: unknown) => void
  api.get.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finish = resolve
      })
  )
  const client = await open()
  const first = within(
    await screen.findByRole('article', { name: 'Billing item log:22816' })
  )
  const second = within(
    screen.getByRole('article', { name: 'Billing item log:2' })
  )
  fireEvent.change(second.getByRole('textbox'), {
    target: { value: 'Customer compensation evidence not yet submitted' },
  })
  fireEvent.change(first.getByLabelText('Handling decision'), {
    target: { value: 'keep_record' },
  })
  fireEvent.click(first.getByRole('button', { name: 'Save handling decision' }))
  await screen.findByText('Checking the full scope. Please wait.')
  expect(second.getByRole('textbox')).toHaveValue(
    'Customer compensation evidence not yet submitted'
  )
  finish({ success: true, data: report })
  await waitFor(() =>
    expect(
      screen.queryByText('Checking the full scope. Please wait.')
    ).toBeNull()
  )
  expect(second.getByRole('textbox')).toHaveValue(
    'Customer compensation evidence not yet submitted'
  )
  client.clear()
})

it('keeps a dirty draft through failed checks and requires review after evidence changes', async () => {
  const report = {
    fingerprint: 'source',
    decision_version: 3,
    rows: 1,
    issues: [issue],
    blockers: 0,
    pending: 1,
    captured_at: 100,
    history: [],
  }
  api.get.mockResolvedValue({ success: true, data: report })
  const client = await open()
  const item = within(
    await screen.findByRole('article', { name: 'Billing item log:22816' })
  )
  fireEvent.change(item.getByRole('textbox'), {
    target: { value: 'Customer dispute remains open' },
  })
  api.get.mockRejectedValueOnce(new Error('unavailable'))
  fireEvent.click(screen.getByRole('button', { name: 'Run checks again' }))
  await screen.findByText(
    'Source checks failed. No completeness conclusion is available.'
  )
  expect(item.getByRole('textbox')).toHaveValue('Customer dispute remains open')
  expect(
    item.getByRole('button', { name: 'Save handling decision' })
  ).toBeDisabled()
  api.get.mockResolvedValue({
    success: true,
    data: { ...report, fingerprint: 'corrected' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Run checks again' }))
  await item.findByText(
    'The records changed. Your input is preserved. Review the latest evidence before submitting.'
  )
  expect(
    item.getByRole('button', { name: 'Save handling decision' })
  ).toBeDisabled()
  fireEvent.click(
    item.getByRole('button', { name: 'I have reviewed the updated records' })
  )
  expect(
    item.getByRole('button', { name: 'Save handling decision' })
  ).toBeEnabled()
  expect(item.getByRole('textbox')).toHaveValue('Customer dispute remains open')
  client.clear()
})

it('protects unsaved input from pagination and retains it if the issue disappears', async () => {
  const report = {
    fingerprint: 'source',
    decision_version: 3,
    rows: 11,
    issues: [
      issue,
      ...Array.from({ length: 10 }, (_, i) => ({ ...issue, id: `log:${i}` })),
    ],
    blockers: 0,
    pending: 11,
    captured_at: 100,
    history: [],
  }
  api.get.mockResolvedValue({ success: true, data: report })
  const client = await open()
  const item = within(
    await screen.findByRole('article', { name: 'Billing item log:22816' })
  )
  fireEvent.change(item.getByRole('textbox'), {
    target: { value: 'Do not lose my note' },
  })
  expect(screen.getByRole('button', { name: /^Next$/ })).toBeDisabled()
  api.get.mockResolvedValue({
    success: true,
    data: { ...report, fingerprint: 'fixed', issues: [] },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Run checks again' }))
  await item.findByText(
    'This item no longer appears in the latest checks. Your unsaved input is kept for reference; discard it when you are ready.'
  )
  expect(item.getByRole('textbox')).toHaveValue('Do not lose my note')
  expect(
    item.getByRole('button', { name: 'Save handling decision' })
  ).toBeDisabled()
  fireEvent.click(item.getByRole('button', { name: 'Discard unsaved changes' }))
  await waitFor(() =>
    expect(
      screen.queryByRole('article', { name: 'Billing item log:22816' })
    ).toBeNull()
  )
  client.clear()
})

it('shows an orphan pending request and submits explicit closure without amounts', async () => {
  api.get.mockResolvedValue({
    success: true,
    data: {
      fingerprint: 'corrected',
      decision_version: 3,
      rows: 1,
      issues: [],
      blockers: 0,
      pending: 1,
      captured_at: 100,
      history: [],
      pending_requests: [
        {
          id: 7,
          issue_id: 'log:22816',
          decision: 'request_waiver',
          model: 'seedance-2-0-oversea',
          recorded_quota: '913500',
          log_type: 2,
          actor_id: 1,
          created_at: 100,
          note: 'Customer requests compensation',
          current_source: false,
        },
      ],
    },
  })
  api.record.mockResolvedValue({ success: true, decision_version: 3 })
  const client = await open()
  const item = within(
    await screen.findByRole('article', { name: 'Open request #7' })
  )
  expect(
    item.getByText(
      'Billing evidence has changed; this request is still pending.'
    )
  ).toBeInTheDocument()
  fireEvent.change(item.getByLabelText('Handling decision'), {
    target: { value: 'reject_request' },
  })
  fireEvent.change(item.getByRole('textbox'), {
    target: { value: 'Reviewed with customer; keep current accounting' },
  })
  fireEvent.click(item.getByRole('button', { name: 'Save handling decision' }))
  await waitFor(() => expect(api.record).toHaveBeenCalledTimes(1))
  expect(api.record.mock.calls[0][1]).toMatchObject({
    fingerprint: 'corrected',
    previous_id: 7,
    decision: 'reject_request',
    reason: 'request_rejected',
  })
  expect(api.record.mock.calls[0][1]).not.toHaveProperty('statement_delta')
  expect(api.record.mock.calls[0][1]).not.toHaveProperty('balance_delta')
  client.clear()
})

it('shows audit damage while disabling all decisions and completeness', async () => {
  api.get.mockResolvedValue({
    success: true,
    data: {
      fingerprint: 'source',
      decision_version: 3,
      rows: 1,
      issues: [issue],
      blockers: 1,
      pending: 1,
      captured_at: 100,
      history: [],
      audit_issues: [{ id: 12, actor_id: 1, created_at: 100 }],
    },
  })
  const client = await open()
  const item = within(
    await screen.findByRole('article', { name: 'Billing item log:22816' })
  )
  expect(
    screen.getByText(/Some handling records are damaged/)
  ).toBeInTheDocument()
  expect(
    item.getByRole('button', { name: 'Save handling decision' })
  ).toBeDisabled()
  fireEvent.click(
    screen.getByRole('button', { name: 'Technical source verification' })
  )
  fireEvent.change(screen.getByLabelText('Backup and coverage evidence'), {
    target: { value: 'backup' },
  })
  fireEvent.change(
    screen.getByLabelText('Log retention and recovery evidence'),
    { target: { value: 'retention' } }
  )
  expect(
    screen.getByRole('button', { name: 'Record verified completeness' })
  ).toBeDisabled()
  client.clear()
})

it('retains a closure draft when another administrator closes the request', async () => {
  const request = {
    id: 7,
    issue_id: 'log:22816',
    decision: 'request_waiver',
    model: 'video',
    recorded_quota: '913500',
    log_type: 2,
    actor_id: 1,
    created_at: 100,
    note: 'Customer request',
    current_source: true,
  }
  const report = {
    fingerprint: 'source',
    decision_version: 3,
    rows: 1,
    issues: [],
    blockers: 0,
    pending: 1,
    captured_at: 100,
    history: [],
    pending_requests: [request],
  }
  api.get.mockResolvedValue({ success: true, data: report })
  const client = await open()
  const item = within(
    await screen.findByRole('article', { name: 'Open request #7' })
  )
  fireEvent.change(item.getByLabelText('Handling decision'), {
    target: { value: 'reject_request' },
  })
  fireEvent.change(item.getByRole('textbox'), {
    target: { value: 'Do not lose this settlement discussion' },
  })
  api.get.mockResolvedValue({
    success: true,
    data: { ...report, pending: 0, pending_requests: [] },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Run checks again' }))
  await waitFor(() =>
    expect(
      screen.queryByText('Checking the full scope. Please wait.')
    ).toBeNull()
  )
  const retained = within(
    screen.getByRole('article', { name: 'Open request #7' })
  )
  expect(retained.getByRole('textbox')).toHaveValue(
    'Do not lose this settlement discussion'
  )
  expect(retained.getByLabelText('Handling decision')).toHaveValue(
    'reject_request'
  )
  expect(
    retained.getByRole('button', { name: 'Save handling decision' })
  ).toBeDisabled()
  fireEvent.click(
    retained.getByRole('button', { name: 'Discard unsaved changes' })
  )
  await waitFor(() =>
    expect(
      screen.queryByRole('article', { name: 'Open request #7' })
    ).toBeNull()
  )
  expect(api.record).not.toHaveBeenCalled()
  client.clear()
})

it('retains an issue draft when refreshed records move it to another page', async () => {
  const issues = Array.from({ length: 21 }, (_, i) => ({
    ...issue,
    id: `log:${i + 1}`,
  }))
  const report = {
    fingerprint: 'source',
    decision_version: 3,
    rows: 21,
    issues,
    blockers: 0,
    pending: 21,
    captured_at: 100,
    history: [],
  }
  api.get.mockResolvedValue({ success: true, data: report })
  api.record.mockResolvedValue({ success: true, decision_version: 3 })
  const client = await open()
  await screen.findByRole('article', { name: 'Billing item log:1' })
  fireEvent.click(screen.getByRole('button', { name: /^Next$/ }))
  const item = within(
    screen.getByRole('article', { name: 'Billing item log:11' })
  )
  fireEvent.change(item.getByLabelText('Handling decision'), {
    target: { value: 'request_waiver' },
  })
  fireEvent.change(item.getByRole('textbox'), {
    target: { value: 'Agreed compensation to review' },
  })
  const second = within(
    screen.getByRole('article', { name: 'Billing item log:12' })
  )
  fireEvent.change(second.getByRole('textbox'), {
    target: { value: 'A different open discussion' },
  })
  api.get.mockResolvedValue({
    success: true,
    data: { ...report, fingerprint: 'updated', issues: issues.slice(1) },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Run checks again' }))
  await waitFor(() =>
    expect(
      screen.queryByText('Checking the full scope. Please wait.')
    ).toBeNull()
  )
  const retained = within(
    screen.getByRole('article', { name: 'Billing item log:11' })
  )
  expect(retained.getByRole('textbox')).toHaveValue(
    'Agreed compensation to review'
  )
  expect(retained.getByLabelText('Handling decision')).toHaveValue(
    'request_waiver'
  )
  expect(retained.getByLabelText('Business reason')).toHaveValue(
    'customer_agreement'
  )
  expect(
    retained.getByRole('button', { name: 'Save handling decision' })
  ).toBeDisabled()
  fireEvent.click(
    retained.getByRole('button', {
      name: 'I have reviewed the updated records',
    })
  )
  expect(
    retained.getByRole('button', { name: 'Save handling decision' })
  ).toBeEnabled()
  fireEvent.click(
    retained.getByRole('button', { name: 'Save handling decision' })
  )
  await waitFor(() => expect(api.record).toHaveBeenCalledTimes(1))
  await waitFor(() =>
    expect(
      screen.queryByRole('article', { name: 'Billing item log:11' })
    ).toBeNull()
  )
  expect(second.getByRole('textbox')).toHaveValue('A different open discussion')
  expect(screen.getByRole('button', { name: 'Previous' })).toBeDisabled()
  fireEvent.click(
    second.getByRole('button', { name: 'Discard unsaved changes' })
  )
  expect(screen.getByRole('button', { name: 'Previous' })).toBeEnabled()
  fireEvent.click(screen.getByRole('button', { name: 'Previous' }))
  expect(
    within(
      screen.getByRole('article', { name: 'Billing item log:11' })
    ).getByRole('textbox')
  ).toHaveValue('')
  client.clear()
})

it('retains a closure draft when earlier requests disappear and change pagination', async () => {
  const requests = Array.from({ length: 21 }, (_, i) => ({
    id: i + 1,
    issue_id: `log:${i + 1}`,
    decision: 'request_waiver',
    model: 'video',
    recorded_quota: '100',
    log_type: 2,
    actor_id: 1,
    created_at: 100,
    note: 'Review request',
    current_source: true,
  }))
  const report = {
    fingerprint: 'source',
    decision_version: 3,
    rows: 21,
    issues: [],
    blockers: 0,
    pending: 21,
    captured_at: 100,
    history: [],
    pending_requests: requests,
  }
  api.get.mockResolvedValue({ success: true, data: report })
  const client = await open()
  await screen.findByRole('article', { name: 'Open request #1' })
  fireEvent.click(screen.getByRole('button', { name: 'Next requests' }))
  const item = within(screen.getByRole('article', { name: 'Open request #11' }))
  fireEvent.change(item.getByRole('textbox'), {
    target: { value: 'Customer has withdrawn the request' },
  })
  api.get.mockResolvedValue({
    success: true,
    data: { ...report, pending_requests: requests.slice(1) },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Run checks again' }))
  await waitFor(() =>
    expect(
      screen.queryByText('Checking the full scope. Please wait.')
    ).toBeNull()
  )
  const retained = within(
    screen.getByRole('article', { name: 'Open request #11' })
  )
  expect(retained.getByRole('textbox')).toHaveValue(
    'Customer has withdrawn the request'
  )
  expect(
    retained.getByRole('button', { name: 'Save handling decision' })
  ).toBeEnabled()
  client.clear()
})
