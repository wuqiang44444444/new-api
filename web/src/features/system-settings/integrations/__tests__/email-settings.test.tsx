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
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { EmailSettingsSection } from '../email-settings-section'

const { put } = vi.hoisted(() => ({ put: vi.fn() }))
vi.mock('@/lib/api', () => ({ api: { put } }))

const defaults = {
  SMTPServer: 'smtp.example.com',
  SMTPPort: '587',
  SMTPAccount: 'sender@example.com',
  SMTPFrom: '',
  SMTPToken: '',
  SMTPSSLEnabled: false,
  SMTPStartTLSEnabled: true,
  SMTPInsecureSkipVerify: false,
  SMTPForceAuthLogin: false,
  'error_report_setting.enabled': true,
  'error_report_setting.recipients': 'ops@example.com',
}
let actions: HTMLDivElement
beforeEach(() => {
  put.mockReset().mockResolvedValue({ data: { success: true, message: '' } })
  actions = document.createElement('div')
  document.body.append(actions)
})
afterEach(() => actions.remove())

function setup(values = defaults) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const view = (next: typeof defaults) => (
    <QueryClientProvider client={client}>
      <SettingsPageProvider actionsContainer={actions}>
        <EmailSettingsSection defaultValues={next} />
      </SettingsPageProvider>
    </QueryClientProvider>
  )
  const result = render(view(values))
  return { refresh: (next: typeof defaults) => result.rerender(view(next)) }
}

describe('SMTP report configuration', () => {
  test('loaded and refreshed report settings remain valid when only SMTP changes', async () => {
    const { refresh } = setup()
    fireEvent.change(screen.getByLabelText('SMTP Host'), {
      target: { value: 'smtp2.example.com' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save SMTP settings' }))
    await waitFor(() =>
      expect(put).toHaveBeenCalledWith('/api/option/', {
        key: 'SMTPServer',
        value: 'smtp2.example.com',
      })
    )
    refresh({
      ...defaults,
      SMTPServer: 'smtp2.example.com',
      'error_report_setting.recipients': 'new@example.com',
    })
    await waitFor(() =>
      expect(screen.getByLabelText('Report recipients')).toHaveValue(
        'new@example.com'
      )
    )
    fireEvent.change(screen.getByLabelText('SMTP Host'), {
      target: { value: 'smtp3.example.com' },
    })
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Save SMTP settings' })
      ).toBeEnabled()
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save SMTP settings' }))
    await waitFor(() =>
      expect(put).toHaveBeenLastCalledWith('/api/option/', {
        key: 'SMTPServer',
        value: 'smtp3.example.com',
      })
    )
  })

  test('a settings refresh during save keeps the form busy until every update completes', async () => {
    const { refresh } = setup()
    let release!: (value: {
      data: { success: boolean; message: string }
    }) => void
    put.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          release = resolve
        })
    )
    fireEvent.change(screen.getByLabelText('SMTP Host'), {
      target: { value: 'smtp2.example.com' },
    })
    fireEvent.change(screen.getByLabelText('Report recipients'), {
      target: { value: 'new@example.com' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save SMTP settings' }))
    await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
    refresh({ ...defaults, SMTPServer: 'smtp2.example.com' })
    expect(screen.getByLabelText('Report recipients')).toHaveValue(
      'new@example.com'
    )
    expect(screen.getByRole('button', { name: 'Saving...' })).toBeDisabled()
    release({ data: { success: true, message: '' } })
    await waitFor(() =>
      expect(put).toHaveBeenLastCalledWith('/api/option/', {
        key: 'error_report_setting.recipients',
        value: 'new@example.com',
      })
    )
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Save SMTP settings' })
      ).toBeEnabled()
    )
  })

  test.each([true, false])(
    'first setup saves prerequisites before enabling; SMTP save success=%s',
    async (success) => {
      setup({
        ...defaults,
        SMTPServer: '',
        SMTPAccount: '',
        'error_report_setting.enabled': false,
        'error_report_setting.recipients': '',
      })
      put.mockResolvedValue({
        data: { success, message: success ? '' : 'save failed' },
      })
      fireEvent.change(screen.getByLabelText('SMTP Host'), {
        target: { value: 'smtp.example.com' },
      })
      fireEvent.change(screen.getByLabelText('Username'), {
        target: { value: 'sender@example.com' },
      })
      fireEvent.click(
        screen.getByRole('switch', { name: 'Hourly system & error report' })
      )
      fireEvent.change(await screen.findByLabelText('Report recipients'), {
        target: { value: 'ops@example.com' },
      })
      fireEvent.click(
        screen.getByRole('button', { name: 'Save SMTP settings' })
      )
      await waitFor(() => expect(put).toHaveBeenCalled())
      await waitFor(() =>
        expect(
          screen.getByRole('button', { name: 'Save SMTP settings' })
        ).toBeEnabled()
      )
      const keys = put.mock.calls.map((call) => call[1].key)
      expect(keys).toEqual(
        success
          ? [
              'SMTPServer',
              'SMTPAccount',
              'error_report_setting.recipients',
              'error_report_setting.enabled',
            ]
          : ['SMTPServer']
      )
    }
  )
})
