/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { EmailSettingsSection } from '../email-settings-section'

const { get, put, post } = vi.hoisted(() => ({
  get: vi.fn(),
  put: vi.fn(),
  post: vi.fn(),
}))
vi.mock('@/lib/api', () => ({ api: { get, put, post } }))

const defaults = {
  'error_report_setting.recipients': '',
  'resource_alert_setting.cpu': 85,
  'resource_alert_setting.memory': 85,
  'resource_alert_setting.disk': 85,
}
const smtp = {
  server: 'smtp.example.com',
  port: 587,
  account: 'sender@example.com',
  from: '',
  ssl: false,
  start_tls: true,
  skip_verify: false,
  force_login: false,
}
let actions: HTMLDivElement
beforeEach(() => {
  get
    .mockReset()
    .mockResolvedValue({
      data: {
        success: true,
        data: { config: { mode: 'smtp', smtp }, has_credential: true },
      },
    })
  put.mockReset().mockImplementation(async (url: string, body: unknown) => ({
    data: {
      success: true,
      data:
        url === '/api/option/mail'
          ? { config: body, has_credential: true }
          : null,
    },
  }))
  post.mockReset().mockResolvedValue({ data: { success: true } })
  actions = document.createElement('div')
  document.body.append(actions)
})
afterEach(() => actions.remove())

async function setup(values = defaults) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <SettingsPageProvider actionsContainer={actions}>
        <EmailSettingsSection defaultValues={values} />
      </SettingsPageProvider>
    </QueryClientProvider>
  )
  return screen.findByRole('button', { name: 'Save email settings' })
}

test('saves the shared report recipients without an enable setting', async () => {
  const save = await setup()
  fireEvent.change(screen.getByLabelText('Report recipients'), {
    target: { value: 'ops@example.com' },
  })
  fireEvent.click(save)
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith('/api/option/', {
      key: 'error_report_setting.recipients',
      value: 'ops@example.com',
    })
  )
  expect(put).toHaveBeenCalledTimes(1)
})

test('saves the selected Graph configuration and shared recipients', async () => {
  const save = await setup()
  fireEvent.click(screen.getByRole('radio', { name: 'Microsoft Graph' }))
  fireEvent.change(screen.getByLabelText('Tenant ID'), {
    target: { value: 'tenant-fixture' },
  })
  fireEvent.change(screen.getByLabelText('Client ID'), {
    target: { value: 'client-fixture' },
  })
  fireEvent.change(screen.getByLabelText('Dedicated sender mailbox'), {
    target: { value: 'sender@example.com' },
  })
  fireEvent.change(screen.getByLabelText('Client Secret'), {
    target: { value: 'synthetic-credential' },
  })
  fireEvent.change(screen.getByLabelText('Report recipients'), {
    target: { value: 'ops@example.com' },
  })
  fireEvent.click(save)
  await waitFor(() => expect(put).toHaveBeenCalledTimes(2))
  expect(put).toHaveBeenNthCalledWith(1, '/api/option/mail', {
    mode: 'graph',
    graph: {
      tenant_id: 'tenant-fixture',
      client_id: 'client-fixture',
      sender: 'sender@example.com',
      client_secret: 'synthetic-credential',
    },
  })
  expect(put).toHaveBeenNthCalledWith(2, '/api/option/', {
    key: 'error_report_setting.recipients',
    value: 'ops@example.com',
  })
})

test('test mail submits only a recipient and uses the saved configuration', async () => {
  await setup()
  fireEvent.change(screen.getByLabelText('Test email recipient'), {
    target: { value: 'inbox@example.com' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Send test email' }))
  await waitFor(() =>
    expect(post).toHaveBeenCalledWith('/api/option/mail/test', {
      recipient: 'inbox@example.com',
    })
  )
  expect(put).not.toHaveBeenCalled()
})
