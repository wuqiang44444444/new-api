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
import {
  createRootRoute,
  createRouter,
  createMemoryHistory,
  RouterProvider,
} from '@tanstack/react-router'
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

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
  useAuthStore.getState().auth.reset()
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
  const router = createRouter({
    routeTree: createRootRoute({ component: ErrorLogViewer }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <RouterProvider router={router} />
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

const TASK_FAILURE_ITEM = {
  ...ERROR_ITEM,
  event_type: 'task_failure',
  status: 0,
  stage: 'task_lifecycle',
  reason: 'task_failed',
  public_code: 'AuditSubmitIllegal',
  task_id: 'vid-task-1',
  request_id: 'req-task-1',
  detail: JSON.stringify({
    platform: 'vidu_modelark_v3',
    fail_reason:
      '输入内容未通过安全审核 (input content failed the upstream safety review)',
  }),
}

it.each(['provider-create-request', ''])(
  'links the failed task and identifies its creation trace (%s)',
  async (requestId) => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: {
          total: 1,
          items: [
            {
              ...TASK_FAILURE_ITEM,
              detail: JSON.stringify({
                create_upstream_request_id: requestId,
                fail_reason: 'Input text may contain sensitive information.',
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
    const link = screen.getByRole('link', { name: 'View task' })
    const destination = new URL(
      link.getAttribute('href') || '',
      'http://localhost'
    )
    expect(destination.pathname).toBe('/usage-logs/task')
    expect(destination.searchParams.get('filter')).toBe('vid-task-1')
    expect(destination.searchParams.get('startTime')).toBe('0')
    expect(screen.getByText('Upstream creation request ID')).toBeVisible()
    expect(screen.getByText(requestId || 'Not recorded')).toBeVisible()
    expect(
      screen.queryByText('create_upstream_request_id')
    ).not.toBeInTheDocument()
    expect(
      screen.getByText('Input text may contain sensitive information.')
    ).toBeVisible()
  }
)

it('offers request evidence instead of four empty body blocks for task failures', async () => {
  const get = vi.spyOn(api, 'get').mockImplementation(async (url: string) => {
    if (url === '/api/task_request_evidence') {
      return { data: { success: true, data: { items: [], total: 0 } } }
    }
    return {
      data: { success: true, data: { total: 1, items: [TASK_FAILURE_ITEM] } },
    }
  })
  await renderViewer()
  await userEvent
    .setup()
    .click(await screen.findByRole('button', { name: 'Details' }))
  expect(
    await screen.findByRole('button', { name: 'Request evidence' })
  ).toBeVisible()
  expect(screen.queryByText('Original request (redacted)')).toBeNull()
  expect(screen.queryByText('Not recorded for this event')).toBeNull()
  expect(get).not.toHaveBeenCalledWith(
    '/api/task_request_evidence',
    expect.anything()
  )
  await userEvent
    .setup()
    .click(screen.getByRole('button', { name: 'Request evidence' }))
  await waitFor(() =>
    expect(get).toHaveBeenCalledWith('/api/task_request_evidence', {
      params: expect.objectContaining({
        task_id: 'vid-task-1',
      }),
    })
  )
  expect(await screen.findByText('No request evidence recorded')).toBeVisible()
})

it.each(['{ }', 'null', '[]', 'invalid JSON', '{"response":null}'])(
  'omits empty HTTP blocks when the task failure snapshot is %s',
  async (httpExchange) => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: {
          total: 1,
          items: [
            {
              ...TASK_FAILURE_ITEM,
              detail: JSON.stringify({ http_exchange: httpExchange }),
            },
          ],
        },
      },
    })
    await renderViewer()
    await userEvent
      .setup()
      .click(await screen.findByRole('button', { name: 'Details' }))
    expect(
      screen.getByRole('button', { name: 'Request evidence' })
    ).toBeVisible()
    expect(
      screen.queryByText('Original request (redacted)')
    ).not.toBeInTheDocument()
    expect(
      screen.queryByText('Not recorded for this event')
    ).not.toBeInTheDocument()
  }
)

it('finds polling evidence without a request ID on a historical task index', async () => {
  vi.spyOn(api, 'get').mockImplementation(async (url, config) => {
    if (url === '/api/task_request_evidence') {
      const matches =
        config?.params?.task_id === 'vid-task-1' && !config?.params?.request_id
      return {
        data: {
          success: true,
          data: {
            items: matches ? [{ id: 9, request_id: '' }] : [],
            total: matches ? 1 : 0,
          },
        },
      }
    }
    if (url === '/api/task_request_evidence/9') {
      return {
        data: {
          success: true,
          data: {
            evidence: { body_expired: false },
            events: [
              {
                id: 10,
                stage: 'polling',
                phase: 'responded',
                complete: true,
                has_body: true,
                body_status: 'available',
                preview:
                  '{"status":"failed","error":{"code":"AuditSubmitIllegal"}}',
                byte_count: 67,
                status_code: 200,
              },
            ],
          },
        },
      }
    }
    return {
      data: { success: true, data: { items: [TASK_FAILURE_ITEM], total: 1 } },
    }
  })
  await renderViewer()
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Details' }))
  await user.click(screen.getByRole('button', { name: 'Request evidence' }))
  await user.click(await screen.findByRole('button', { name: '9' }))
  expect(await screen.findByText(/Task polling/)).toBeVisible()
  expect(screen.getByText(/"status":"failed"/)).toBeVisible()
  expect(screen.queryByText('No request evidence recorded')).toBeNull()
})

it('uses the request ID when the failure has no task ID', async () => {
  const get = vi.spyOn(api, 'get').mockImplementation(async (url) => ({
    data: {
      success: true,
      data:
        url === '/api/task_request_evidence'
          ? { items: [], total: 0 }
          : { items: [{ ...TASK_FAILURE_ITEM, task_id: '' }], total: 1 },
    },
  }))
  await renderViewer()
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Details' }))
  await user.click(screen.getByRole('button', { name: 'Request evidence' }))
  await waitFor(() =>
    expect(get).toHaveBeenCalledWith('/api/task_request_evidence', {
      params: {
        task_id: undefined,
        request_id: 'req-task-1',
        p: 1,
        page_size: 20,
      },
    })
  )
})

it.each(['en', 'zh'])(
  'shows dedicated translated task failure fields in %s',
  async (language) => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: { items: [TASK_FAILURE_ITEM], total: 1 } },
    })
    await renderViewer(language)
    await userEvent.setup().click(
      await screen.findByRole('button', {
        name: language === 'zh' ? '详情' : 'Details',
      })
    )
    const dialog = screen.getByRole('dialog')
    expect(
      within(dialog).getByText(language === 'zh' ? '错误码' : 'Error code')
    ).toBeVisible()
    expect(
      within(dialog).getByText(
        language === 'zh' ? '失败原因' : 'Failure reason'
      )
    ).toBeVisible()
    expect(
      within(dialog).getByText('AuditSubmitIllegal', { exact: true })
    ).toBeVisible()
    expect(within(dialog).queryByText('fail_reason')).toBeNull()
    expect(
      within(dialog).getByText(
        language === 'zh' ? '上游创建请求 ID' : 'Upstream creation request ID'
      )
    ).toBeVisible()
    expect(
      within(dialog).getByRole('link', {
        name: language === 'zh' ? '查看任务' : 'View task',
      })
    ).toBeVisible()
  }
)

it('offers an evidence lookup for linked Midjourney failures without empty HTTP blocks', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        items: [
          {
            ...TASK_FAILURE_ITEM,
            stage: 'midjourney',
            detail: JSON.stringify({
              platform: 'midjourney',
              fail_reason: 'Generation failed',
            }),
          },
        ],
        total: 1,
      },
    },
  })
  await renderViewer()
  await userEvent
    .setup()
    .click(await screen.findByRole('button', { name: 'Details' }))
  expect(screen.getByRole('button', { name: 'Request evidence' })).toBeVisible()
  expect(screen.queryByText('Not recorded for this event')).toBeNull()
  expect(screen.getByText('Generation failed')).toBeVisible()
})

it('keeps the body snapshot view and adds the evidence entry for task failures that recorded one', async () => {
  vi.spyOn(api, 'get').mockImplementation(async (url: string) => {
    if (url === '/api/task_request_evidence') {
      return { data: { success: true, data: { items: [], total: 0 } } }
    }
    return {
      data: {
        success: true,
        data: {
          total: 1,
          items: [
            {
              ...TASK_FAILURE_ITEM,
              detail: JSON.stringify({
                platform: 'vidu_modelark_v3',
                http_exchange: JSON.stringify({
                  response: {
                    state: 'captured',
                    status: 200,
                    body: '{"error":{"code":"generation_failed"}}',
                  },
                }),
              }),
            },
          ],
        },
      },
    }
  })
  await renderViewer()
  await userEvent
    .setup()
    .click(await screen.findByRole('button', { name: 'Details' }))
  expect(await screen.findByText('Gateway response (redacted)')).toBeVisible()
  // The sanitized HTTP snapshot no longer blocks the associated evidence view.
  expect(screen.getByRole('button', { name: 'Request evidence' })).toBeVisible()
  await userEvent
    .setup()
    .click(screen.getByRole('button', { name: 'Request evidence' }))
  await waitFor(() =>
    expect(api.get).toHaveBeenCalledWith('/api/task_request_evidence', {
      params: expect.objectContaining({ task_id: 'vid-task-1' }),
    })
  )
})

it('keeps the capture failure state when a task snapshot has no body', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        total: 1,
        items: [
          {
            ...TASK_FAILURE_ITEM,
            detail: JSON.stringify({
              http_exchange: JSON.stringify({
                response: { state: 'read_failed' },
              }),
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
  expect(screen.getByText('Gateway response (redacted)')).toBeVisible()
  expect(screen.getByText('Body could not be read completely')).toBeVisible()
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

it.each(['en', 'zh'])(
  'lets Root explicitly view associated original text from an ordinary error in %s',
  async (language) => {
    useAuthStore
      .getState()
      .auth.setUser({ id: 7, username: 'root', role: ROLE.SUPER_ADMIN })
    const original = '{"url":"https://example.test/file?sig=fixture"}'
    const get = vi.spyOn(api, 'get').mockImplementation(async (url) => {
      if (url === '/api/error_log/') {
        return {
          data: { success: true, data: { items: [ERROR_ITEM], total: 1 } },
        }
      }
      if (url === '/api/task_request_evidence') {
        return {
          data: {
            success: true,
            data: { items: [{ id: 9, request_id: 'req-e1' }], total: 1 },
          },
        }
      }
      if (url === '/api/task_request_evidence/9') {
        return {
          data: {
            success: true,
            data: {
              evidence: { body_expired: false },
              events: [
                {
                  id: 10,
                  stage: 'upstream_response',
                  complete: true,
                  has_body: true,
                  body_status: 'available',
                  preview: 'masked preview',
                  status_code: 400,
                  byte_count: 32,
                },
              ],
            },
          },
        }
      }
      if (url === '/api/task_request_evidence/9/events/10/content') {
        return { data: original }
      }
      throw new Error('Unexpected URL')
    })
    await renderViewer(language)
    const user = userEvent.setup()
    await user.click(
      await screen.findByRole('button', {
        name: language === 'zh' ? '详情' : 'Details',
      })
    )
    await user.click(
      screen.getByRole('button', {
        name: language === 'zh' ? '请求证据' : 'Request evidence',
      })
    )
    await user.click(await screen.findByRole('button', { name: 'req-e1' }))
    expect(await screen.findByText('masked preview')).toBeVisible()
    expect(get.mock.calls.some(([url]) => url.endsWith('/content'))).toBe(false)
    await user.click(
      screen.getByRole('button', {
        name: language === 'zh' ? '查看原文' : 'View original text',
      })
    )
    expect(await screen.findByText(original)).toBeVisible()
    expect(
      screen.getByRole('dialog', {
        name: language === 'zh' ? '查看原文' : 'View original text',
      })
    ).toHaveAccessibleDescription(
      language === 'zh'
        ? '认证凭据已移除'
        : 'Authentication credentials removed'
    )
  }
)
