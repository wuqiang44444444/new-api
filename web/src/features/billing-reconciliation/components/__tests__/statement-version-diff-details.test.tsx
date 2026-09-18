import { render, screen, within } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { expect, it } from 'vitest'

import { StatementVersionDiffDetails } from '../statement-version-diff-details'

it('shows usage and quality changes when the billed amount has not changed', async () => {
  const i18n = createInstance().use(initReactI18next)
  await i18n.init({ lng: 'en', resources: { en: { translation: {} } } })
  const view = render(
    <I18nextProvider i18n={i18n}>
      <StatementVersionDiffDetails
        diff={{
          base_version_id: 1,
          compare_version_id: 2,
          items: {},
          groups: [],
          models: [],
          usage: {
            input_tokens: {
              base: null,
              compare: '9007199254740993',
              delta: null,
            },
          },
          quality: {
            input_tokens_unavailable_requests: {
              base: 1,
              compare: 0,
              delta: -1,
            },
          },
          discounts: { base: [], compare: [] },
        }}
      />
    </I18nextProvider>
  )
  const usage = screen.getByRole('table', { name: 'Usage changes' })
  expect(within(usage).getByText('Unavailable')).toBeTruthy()
  expect(within(usage).getByText('9007199254740993')).toBeTruthy()
  const quality = screen.getByRole('table', { name: 'Quality changes' })
  expect(within(quality).getByText('-1')).toBeTruthy()
  view.unmount()
})

it.each([
  ['unrecorded', 'Not recorded'],
  ['no', 'Not applicable'],
] as const)(
  'explains absent contract ratios for %s instead of marking them unavailable',
  async (state, label) => {
    const i18n = createInstance().use(initReactI18next)
    await i18n.init({ lng: 'en', resources: { en: { translation: {} } } })
    const view = render(
      <I18nextProvider i18n={i18n}>
        <StatementVersionDiffDetails
          diff={{
            base_version_id: 1,
            compare_version_id: 2,
            items: {},
            groups: [],
            models: [],
            usage: {},
            quality: {},
            discounts: {
              base: [],
              compare: [
                {
                  group_id: 4,
                  model_name: 'model',
                  billing_mode: 'token',
                  group_ratio: 1,
                  contract_applicable: state,
                  original_known: true,
                  usage: {
                    requests: 1,
                    billable_calls: 0,
                    refunded_calls: 0,
                    input_tokens: 1,
                    output_tokens: 0,
                    cache_read_tokens: 0,
                    cache_write_tokens: 0,
                    gross_quota: 1,
                    refund_quota: 0,
                    net_quota: 1,
                  },
                },
              ],
            },
          }}
        />
      </I18nextProvider>
    )
    const table = screen.getByRole('table', {
      name: 'Discount combinations after correction',
    })
    expect(within(table).getAllByText(label)).toHaveLength(2)
    expect(within(table).queryByText('Unavailable')).toBeNull()
    view.unmount()
  }
)
