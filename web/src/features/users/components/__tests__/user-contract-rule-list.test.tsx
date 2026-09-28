import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import type { ComponentProps } from 'react'
import { I18nextProvider } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'

import { templateRuleToDraft } from '@/features/customer-contracts/template-utils'
import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'

import type { CustomerContractCatalog } from '../../types'
import { CustomerContractRuleList } from '../user-contract-rule-list'

const emptyCatalog: CustomerContractCatalog = {
  groups: [],
  no_group_channels: [],
  customer_context: true,
}

function unavailableRuleProps(
  reason: string
): ComponentProps<typeof CustomerContractRuleList> {
  return {
    rules: [
      templateRuleToDraft(
        {
          public_model: 'model-a',
          channel_id: 42,
          route_group: 'default',
          ratio_units: 80_000_000,
          available: false,
          unavailable_reason: reason,
        },
        emptyCatalog
      ),
    ],
    catalog: emptyCatalog,
    search: '',
    onSearchChange: vi.fn(),
    onUpdate: vi.fn(),
    onRemove: vi.fn(),
  }
}

describe('unavailable contract sources', () => {
  it.each([
    ['en', 'This channel no longer exists'],
    ['zh', '此渠道已不存在'],
  ])('shows a translated source reason in %s', async (language, message) => {
    const i18n = createInstance()
    await i18n.init({ lng: language, fallbackLng: 'en', resources: { en, zh } })
    render(
      <I18nextProvider i18n={i18n}>
        <CustomerContractRuleList
          {...unavailableRuleProps('channel_missing')}
        />
      </I18nextProvider>
    )
    expect(screen.getByRole('combobox')).toHaveAccessibleDescription(message)
  })

  it.each([
    ['channel_disabled', 'This channel is disabled'],
    ['channel_missing', 'This channel no longer exists'],
    [
      'capability_missing',
      'This channel no longer serves this model in the route group',
    ],
    ['route_group_invalid', 'This route group is no longer configured'],
    [
      'other',
      'This source is unavailable. Review its channel, model, and route group.',
    ],
  ])(
    'shows the %s reason and retains the original source',
    (reason, message) => {
      render(<CustomerContractRuleList {...unavailableRuleProps(reason)} />)
      expect(screen.getByText('model-a')).toBeVisible()
      expect(screen.getByText('default')).toBeVisible()
      expect(screen.getByRole('combobox')).toHaveAccessibleDescription(message)
      expect(screen.getByText(message)).toBeVisible()
      expect(
        screen.getByRole('button', { name: 'Remove contract rule' })
      ).toBeEnabled()
    }
  )

  it('clears the old reason when the administrator selects a qualifying replacement', async () => {
    const props = unavailableRuleProps('channel_disabled')
    props.catalog = {
      groups: [
        {
          group: 'default',
          ratio_configured: true,
          native_group_ratio: '1',
          special_group_ratio: false,
          models: [
            {
              model: 'model-a',
              sources: [
                {
                  channel_id: 43,
                  channel_name: 'Replacement',
                  available: true,
                  from_channel_config: true,
                  from_ability: true,
                },
              ],
            },
          ],
        },
      ],
      no_group_channels: [],
      customer_context: true,
    }
    const { rerender } = render(<CustomerContractRuleList {...props} />)
    const user = userEvent.setup()
    await user.click(screen.getByRole('combobox'))
    await user.click(screen.getByRole('option', { name: 'Replacement' }))
    expect(props.onUpdate).toHaveBeenCalledWith(0, {
      channel_id: 43,
      available: true,
      unavailable_reason: undefined,
    })
    rerender(
      <CustomerContractRuleList
        {...props}
        rules={[
          {
            ...props.rules[0],
            channel_id: 43,
            available: true,
            unavailable_reason: undefined,
          },
        ]}
      />
    )
    expect(
      screen.queryByText('This channel is disabled')
    ).not.toBeInTheDocument()
    expect(screen.getByRole('combobox')).toHaveTextContent('Replacement')
  })
})
