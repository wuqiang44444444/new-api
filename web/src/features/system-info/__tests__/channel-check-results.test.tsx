import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, expect, it, vi } from 'vitest'

import { ChannelCheckDetails } from '@/features/channels/components/channel-check-details'
import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'
import { api } from '@/lib/api'

import { SystemTasksPanel } from '../components/system-tasks-panel'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

it.each([
  ['en', undefined],
  ['zh', 'unknown_configuration_reason'],
])(
  'does not invent a billing diagnosis when the reason is unavailable in %s',
  async (language, reason) => {
    const i18n = createInstance()
    await i18n.init({
      lng: language,
      fallbackLng: false,
      resources: { en, zh },
    })
    const detail: Record<string, string> = {
      config_summary: 'fixture-private-response',
    }
    if (reason) detail.config_reason = reason
    render(
      <I18nextProvider i18n={i18n}>
        <ChannelCheckDetails detail={detail} />
      </I18nextProvider>
    )
    expect(
      screen.getByText(
        i18n.t(
          'Configuration diagnostic unavailable. Review the failure reason and channel settings.'
        )
      )
    ).toBeVisible()
    expect(
      screen.queryByText(
        i18n.t('Billing validation failed. Check the billing model settings.')
      )
    ).not.toBeInTheDocument()
    expect(
      screen.queryByText('fixture-private-response')
    ).not.toBeInTheDocument()
  }
)

it('shows per-channel config recovery with generation still unverified in task history', async () => {
  const i18n = createInstance()
  await i18n.init({ lng: 'en', fallbackLng: false, resources: { en, zh } })
  const detail = {
    check_scope: 'config_only',
    config_check: 'passed',
    readonly_check: 'unsupported',
    upstream_request: 'not_sent',
    generation_evidence: 'not_verified',
    billing_model: 'customer-image',
  }
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: [
        {
          id: 1,
          task_id: 'st-check',
          type: 'channel_test',
          status: 'succeeded',
          created_at: 1,
          updated_at: 2,
          result: {
            checks: [
              { channel_id: 7, model: 'customer-image', checked_at: 2, detail },
            ],
          },
        },
      ],
    },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18n}>
        <SystemTasksPanel />
      </I18nextProvider>
    </QueryClientProvider>
  )
  const user = userEvent.setup()
  const trigger = await screen.findByRole('button', { name: 'Check results' })
  trigger.focus()
  await user.keyboard('{Enter}')
  const dialog = await screen.findByRole('dialog', {
    name: 'Automatic channel check results',
  })
  expect(within(dialog).getByText('Passed')).toBeInTheDocument()
  expect(within(dialog).getByText('Not verified this run')).toBeInTheDocument()
  expect(
    within(dialog).getByText('Read-only check unsupported')
  ).toBeInTheDocument()
  await user.keyboard('{Escape}')
  expect(trigger).toHaveFocus()
  client.clear()
})

it.each(['en', 'zh'])(
  'renders safe translated diagnostics and a fixed management destination in %s',
  async (language) => {
    const i18n = createInstance()
    await i18n.init({
      lng: language,
      fallbackLng: false,
      resources: { en, zh },
    })
    render(
      <I18nextProvider i18n={i18n}>
        <ChannelCheckDetails
          detail={{
            config_reason: 'billing_expr_failed',
            config_summary: 'fixture-secret https://private.example',
            config_entry: 'billing_expression',
            billing_model: 'customer-billing-model',
          }}
        />
      </I18nextProvider>
    )
    expect(
      screen.getByText(
        i18n.t('Check the billing expression and its required inputs.')
      )
    ).toBeInTheDocument()
    expect(screen.queryByText(/fixture-secret/)).not.toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: i18n.t('Billing expression settings') })
    ).toHaveAttribute('href', '/system-settings/billing/model-pricing')
    expect(screen.getByText('customer-billing-model')).toBeInTheDocument()
  }
)

it.each([
  ['en', 'test_upstream_rejected', 'Upstream rejected the probe', '401'],
  ['zh', 'test_upstream_rejected', 'Upstream rejected the probe', '500'],
  ['en', 'response_time_exceeded', 'Response time threshold exceeded', '200'],
  ['zh', 'test_upstream_unreachable', 'Upstream connection failed', undefined],
])(
  'separates a failed probe from passed configuration in %s (%s)',
  async (language, reason, label, status) => {
    const i18n = createInstance()
    await i18n.init({
      lng: language,
      fallbackLng: false,
      resources: { en, zh },
    })
    const detail: Record<string, string> = {
      check_result: 'failed',
      check_reason: reason,
      config_check: 'passed',
      upstream_request: status ? 'response_received' : 'attempted',
    }
    if (status) detail.upstream_status = status
    render(
      <I18nextProvider i18n={i18n}>
        <ChannelCheckDetails detail={detail} />
      </I18nextProvider>
    )
    expect(screen.getByText(i18n.t('Failed'))).toBeVisible()
    expect(screen.getByText(i18n.t('Passed'))).toBeVisible()
    expect(screen.getByText(i18n.t(label))).toBeVisible()
    if (status) expect(screen.getByText(status)).toBeVisible()
  }
)
