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
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'
import { api } from '@/lib/api'

import { ErrorLogViewer } from '../components/error-log-viewer'

const ERROR_ITEM = {
  id: 1,
  created_at: 1758000000,
  module: 'relay',
  event_type: 'api_error',
  task_id: '',
  method: 'POST',
  route: '/v1/chat/completions',
  status: 400,
  user_id: 11,
  username: 'customer_a',
  token_name: 'key-prod-1',
  model_name: 'gpt-4o',
  channel_id: 3,
  channel_name: 'primary',
  request_id: 'req-e1',
  upstream_request_id: '',
  stage: 'relay',
  reason: 'new_api_error',
  public_code: 'invalid_request',
  protocol: '',
  elapsed_ms: 120,
  detail: '{"source_status":"400"}',
}

beforeEach(() => {
  vi.stubGlobal('localStorage', {
    getItem: () => null,
    setItem: () => undefined,
    removeItem: () => undefined,
  })
})
afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

async function renderViewer(lng = 'en') {
  const i18n = createInstance()
  await i18n.init({
    lng,
    fallbackLng: false,
    resources: { en, zh },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <ErrorLogViewer />
      </QueryClientProvider>
    </I18nextProvider>
  )
}

it('renders API error events with status, model and request id', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { total: 1, items: [ERROR_ITEM] } },
  })
  await renderViewer()
  expect(get).toHaveBeenCalledWith('/api/error_log/', {
    params: expect.objectContaining({ p: 1, page_size: 20 }),
  })
  expect(await screen.findByRole('cell', { name: '400' })).toBeVisible()
  expect(screen.getByRole('cell', { name: 'gpt-4o' })).toBeVisible()
  expect(screen.getByRole('cell', { name: 'req-e1' })).toBeVisible()
})

it('passes request id filter to the error log API', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { items: [], total: 0 } },
  })
  await renderViewer()
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Expand' }))
  const input = await screen.findByPlaceholderText('Request ID')
  await user.type(input, 'req-e1')
  await user.click(screen.getByRole('button', { name: 'Search' }))
  await waitFor(() =>
    expect(get).toHaveBeenLastCalledWith('/api/error_log/', {
      params: expect.objectContaining({ request_id: 'req-e1', p: 1 }),
    })
  )
})

it('sends the selected HTTP status and replaces the displayed results on search and reset', async () => {
  const upstreamFailure = {
    ...ERROR_ITEM,
    event_type: 'channel_test',
    status: 200,
    detail: '{"upstream_status":"500","test_mode":"manual"}',
  }
  const get = vi.spyOn(api, 'get').mockImplementation(async (_url, config) => {
    // The persisted filtering contract is covered by the backend integration test.
    const items = config?.params?.status === '200' ? [] : [upstreamFailure]
    return { data: { success: true, data: { items, total: items.length } } }
  })
  await renderViewer()
  const user = userEvent.setup()
  expect(
    await screen.findByRole('cell', { name: 'Upstream HTTP 500' })
  ).toBeVisible()
  await user.click(screen.getByRole('combobox', { name: 'HTTP' }))
  await user.click(await screen.findByRole('option', { name: '200' }))
  await user.click(screen.getByRole('button', { name: 'Search' }))
  await waitFor(() =>
    expect(get).toHaveBeenLastCalledWith('/api/error_log/', {
      params: expect.objectContaining({ status: '200', p: 1 }),
    })
  )
  expect(
    screen.queryByRole('cell', { name: 'Upstream HTTP 500' })
  ).not.toBeInTheDocument()
  await user.click(screen.getByRole('combobox', { name: 'HTTP' }))
  await user.click(await screen.findByRole('option', { name: '500' }))
  expect(
    await screen.findByRole('cell', { name: 'Upstream HTTP 500' })
  ).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Reset' }))
  await waitFor(() =>
    expect(get).toHaveBeenLastCalledWith('/api/error_log/', {
      params: { p: 1, page_size: 20 },
    })
  )
})

it('shows sanitized request and upstream response in channel test details', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        total: 1,
        items: [
          {
            ...ERROR_ITEM,
            event_type: 'channel_test',
            status: 0,
            detail: JSON.stringify({
              test_mode: 'auto',
              upstream_status: '400',
              http_exchange: JSON.stringify({
                request: {
                  state: 'captured',
                  body: '{"prompt":"draw a cat","api_key":"[REDACTED]"}',
                },
                upstream_request: {
                  state: 'captured',
                  body: '{"model":"provider-model","n":1}',
                },
                upstream_response: {
                  state: 'captured',
                  status: 400,
                  body: '{"error":{"message":"unsupported image size"}}',
                },
                response: { state: 'not_recorded' },
              }),
            }),
          },
        ],
      },
    },
  })
  await renderViewer()
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Details' }))
  expect(await screen.findByText('Original request (redacted)')).toBeVisible()
  expect(screen.getByText('Upstream request (redacted)')).toBeVisible()
  expect(screen.getByText('Upstream response (redacted)')).toBeVisible()
  expect(screen.getByText('Gateway response (redacted)')).toBeVisible()
  expect(screen.queryByText('No HTTP response')).not.toBeInTheDocument()
  expect(screen.getAllByText('Upstream HTTP 400')).toHaveLength(2)
  expect(screen.getByText('HTTP 400')).toBeVisible()
  expect(screen.queryByText('HTTP 0')).not.toBeInTheDocument()
  expect(screen.getByText('Not recorded for this event')).toBeVisible()
  expect(
    screen.getByText('unsupported image size', { exact: false })
  ).toBeVisible()
  expect(screen.queryByText('http_exchange')).not.toBeInTheDocument()
  await user.click(screen.getAllByRole('button', { name: 'Copy code' })[0])
  expect(await navigator.clipboard.readText()).toContain('draw a cat')
  expect(await navigator.clipboard.readText()).toContain('[REDACTED]')
})

it('keeps historical error details readable when no bodies were recorded', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { total: 1, items: [ERROR_ITEM] } },
  })
  await renderViewer()
  await userEvent
    .setup()
    .click(await screen.findByRole('button', { name: 'Details' }))
  expect(await screen.findByText('Original request (redacted)')).toBeVisible()
  expect(screen.getAllByText('Not recorded for this event')).toHaveLength(4)
  expect(screen.getByText('source_status')).toBeVisible()
})

it.each([
  {
    name: 'historical automatic rejection',
    status: 0,
    upstream: '400',
    label: 'Upstream HTTP 400',
  },
  {
    name: 'manual rejection',
    status: 200,
    upstream: '503',
    label: 'Upstream HTTP 503',
  },
  {
    name: 'failure before a response',
    status: 0,
    upstream: undefined,
    label: 'Upstream HTTP status not recorded',
  },
])(
  'distinguishes upstream status for $name without body snapshots',
  async (test) => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: {
          total: 1,
          items: [
            {
              ...ERROR_ITEM,
              event_type: 'channel_test',
              status: test.status,
              public_code: '400',
              detail: JSON.stringify({
                test_mode: test.status ? 'manual' : 'auto',
                upstream_status: test.upstream,
              }),
            },
          ],
        },
      },
    })
    await renderViewer()
    expect(await screen.findByRole('cell', { name: test.label })).toBeVisible()
    await userEvent
      .setup()
      .click(screen.getByRole('button', { name: 'Details' }))
    expect(screen.getAllByText(test.label)).toHaveLength(2)
    expect(screen.queryByText('No HTTP response')).not.toBeInTheDocument()
    expect(screen.getAllByText('Not recorded for this event')).toHaveLength(4)
    if (test.status > 0) {
      expect(screen.getByText('Management HTTP 200')).toBeVisible()
    }
  }
)

it('shows upstream HTTP status in Chinese for historical automatic failures', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        total: 1,
        items: [
          {
            ...ERROR_ITEM,
            event_type: 'channel_test',
            status: 0,
            detail: JSON.stringify({
              test_mode: 'auto',
              upstream_status: '400',
            }),
          },
        ],
      },
    },
  })
  await renderViewer('zh')
  expect(
    await screen.findByRole('cell', { name: '上游 HTTP 400' })
  ).toBeVisible()
  await userEvent
    .setup()
    .click(screen.getByRole('button', { name: zh.translation.Details }))
  expect(screen.getAllByText('上游 HTTP 400')).toHaveLength(2)
  expect(screen.queryByText('无 HTTP 响应')).not.toBeInTheDocument()
})

it('keeps automatic-check key names visible in additional info for other event types', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        total: 1,
        items: [
          {
            ...ERROR_ITEM,
            detail: JSON.stringify({
              config_check: 'other-event-value',
              test_mode: 'auto',
            }),
          },
        ],
      },
    },
  })
  await renderViewer()
  await userEvent
    .setup()
    .click(await screen.findByRole('button', { name: 'Details' }))
  expect(screen.getByText('other-event-value')).toBeVisible()
  expect(screen.queryByText('Automatic check')).not.toBeInTheDocument()
})

it.each([
  [
    400,
    'bad_response_status_code',
    'Response received; probe request rejected',
  ],
  [400, 'model_not_supported', 'Response received; probe request rejected'],
  [400, 'unknown_error', 'Response received; probe request rejected'],
  [405, 'unknown_error', 'Response received; probe not applicable'],
  [
    401,
    'unknown_error',
    'Response received; authentication or permission issue',
  ],
  [
    403,
    'unknown_error',
    'Response received; authentication or permission issue',
  ],
  [429, 'unknown_error', 'Response received; rate limited'],
  [503, 'unknown_error', 'Service or gateway issue'],
  [504, 'unknown_error', 'Service or gateway issue'],
])(
  'distinguishes media probe HTTP %s from customer errors',
  async (status, code, message) => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: {
          total: 1,
          items: [
            {
              ...ERROR_ITEM,
              event_type: 'channel_test',
              model_name: 'gpt-image-2',
              public_code: code,
              status: 0,
              detail: JSON.stringify({
                test_mode: 'auto',
                check_scope: 'readonly_probe',
                probe_media: 'image',
                upstream_status: String(status),
              }),
            },
          ],
        },
      },
    })
    await renderViewer()
    expect(await screen.findByText('Automatic connection probe')).toBeVisible()
    expect(screen.getByText('Not a customer request')).toBeVisible()
    expect(screen.getByText('Image probe')).toBeVisible()
    expect(screen.getByText(message)).toBeVisible()
    expect(screen.getByText(`Upstream HTTP ${status}`)).toBeVisible()
  }
)

it('preserves automatic text probe presentation', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        total: 1,
        items: [
          {
            ...ERROR_ITEM,
            event_type: 'channel_test',
            status: 0,
            detail: JSON.stringify({
              test_mode: 'auto',
              check_scope: 'generation_probe',
              upstream_status: '400',
            }),
          },
        ],
      },
    },
  })
  await renderViewer()
  expect(await screen.findByText('Channel Test')).toBeVisible()
  expect(screen.getByText('Upstream HTTP 400')).toBeVisible()
  expect(screen.queryByText('Not a customer request')).not.toBeInTheDocument()
})

it('labels historical video probes and explains their scope in Chinese', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        total: 1,
        items: [
          {
            ...ERROR_ITEM,
            event_type: 'channel_test',
            model_name: 'Seedance2.0',
            status: 0,
            detail: JSON.stringify({
              test_mode: 'auto',
              check_scope: 'generation_probe',
              upstream_status: '405',
            }),
          },
        ],
      },
    },
  })
  await renderViewer('zh')
  expect(await screen.findByText('自动生成探测（历史记录）')).toBeVisible()
  expect(screen.getByText('非客户业务请求')).toBeVisible()
  expect(screen.getByText('已收到响应，探测请求不适用')).toBeVisible()
  await userEvent.setup().click(screen.getByRole('button', { name: '详情' }))
  expect(
    await screen.findByText(
      '这是系统自动探测，非客户业务请求，不代表图片或视频生成失败；收到响应也不代表生成能力正常。'
    )
  ).toBeVisible()
})

it.each([
  ['en', 'gpt-image-2'],
  ['zh', 'Seedance2.0'],
])(
  'labels old media events without inventing a generation scope in %s',
  async (language, model) => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: {
          total: 1,
          items: [
            {
              ...ERROR_ITEM,
              event_type: 'channel_test',
              model_name: model,
              public_code: 'model_not_supported',
              status: 0,
              detail: JSON.stringify({
                test_mode: 'auto',
                upstream_status: '400',
              }),
            },
          ],
        },
      },
    })
    await renderViewer(language)
    const translations = language === 'zh' ? zh.translation : en.translation
    expect(
      await screen.findByText(translations['Automatic probe (historical)'])
    ).toBeVisible()
    expect(
      screen.queryByText(
        translations['Automatic generation probe (historical)']
      )
    ).not.toBeInTheDocument()
    expect(
      screen.getByText(translations['Response received; probe not applicable'])
    ).toBeVisible()
    await userEvent
      .setup()
      .click(screen.getByRole('button', { name: translations.Details }))
    expect(
      screen.getAllByText(translations['Automatic probe (historical)'])
    ).toHaveLength(2)
  }
)

it.each([
  ['gpt-4o', 'auto'],
  ['gpt-image-2', 'manual'],
])(
  'keeps unscoped %s / %s events outside media auto labels',
  async (model, mode) => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: {
          total: 1,
          items: [
            {
              ...ERROR_ITEM,
              event_type: 'channel_test',
              model_name: model,
              detail: JSON.stringify({
                test_mode: mode,
                upstream_status: '400',
              }),
            },
          ],
        },
      },
    })
    await renderViewer()
    expect(await screen.findByText('Channel Test')).toBeVisible()
    expect(
      screen.queryByText('Automatic probe (historical)')
    ).not.toBeInTheDocument()
    expect(screen.queryByText('Not a customer request')).not.toBeInTheDocument()
  }
)
