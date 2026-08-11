import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { useForm } from 'react-hook-form'
import { I18nextProvider } from 'react-i18next'
import { beforeAll, describe, expect, test } from 'vitest'

import { Form } from '@/components/ui/form'

import {
  CHANNEL_FORM_DEFAULT_VALUES,
  type ChannelFormValues,
} from '../../../lib/channel-form'
import { SeedanceProtocolFields } from '../seedance-protocol-fields'

beforeAll(async () => {
  await i18next.init({
    lng: 'en',
    fallbackLng: 'en',
    resources: {
      en: {
        translation: {
          'Seedance Video Protocol': 'Seedance Video Protocol',
          'Seedance Asset Protocol': 'Seedance Asset Protocol',
          'Volcengine ModelArk V3': 'Volcengine ModelArk V3',
          'BytePlus ModelArk V3': 'BytePlus ModelArk V3',
          'Media Task V1': 'Moxing',
          'Ark Media V1': 'Ark Proxy Video API',
          'URL Media Arrays (URL Only, No Asset Library)':
            'URL Media Arrays (URL Only, No Asset Library)',
          'FunCloud Seedance V2': 'FunCloud',
          'Volcengine Official Assets': 'Volcengine Official Assets',
          'BytePlus Official Assets': 'BytePlus Official Assets',
          'No Asset Protocol': 'No Asset Protocol',
          'Relay Assets V1': 'Moxing Asset Library',
          'Ark Assets V1': 'Ark Proxy Asset Library',
        },
      },
    },
  })
})

function SeedanceProtocolFieldsHarness() {
  const form = useForm<ChannelFormValues>({
    defaultValues: {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      type: 61,
      video_upstream_protocol: 'modelark_v3_volcengine',
      asset_upstream_protocol: 'volcengine_assets_action_v2024_01_01',
    },
  })

  return (
    <I18nextProvider i18n={i18next}>
      <Form {...form}>
        <SeedanceProtocolFields
          control={form.control}
          sensitiveLocked={false}
        />
      </Form>
    </I18nextProvider>
  )
}

describe('Seedance protocol fields', () => {
  test('uses wide selectors and links the selected video provider to its asset library', async () => {
    const user = userEvent.setup()
    render(<SeedanceProtocolFieldsHarness />)

    const videoTrigger = screen.getByRole('combobox', {
      name: 'Seedance Video Protocol',
    })
    const assetTrigger = screen.getByRole('combobox', {
      name: 'Seedance Asset Protocol',
    })
    expect(videoTrigger.className).toContain('max-w-2xl')
    expect(assetTrigger.className).toContain('max-w-2xl')
    expect(assetTrigger.textContent).toContain('Volcengine Official Assets')

    await user.click(assetTrigger)
    const initialListbox = await screen.findByRole('listbox')
    const assetPopup = document.querySelector<HTMLElement>(
      '[data-slot="select-content"]'
    )
    expect(assetPopup?.className).toContain('sm:min-w-[36rem]')
    expect(
      within(initialListbox).getByRole('option', {
        name: 'Volcengine Official Assets',
      })
    ).toBeTruthy()
    expect(
      within(initialListbox).queryByRole('option', {
        name: 'BytePlus Official Assets',
      })
    ).toBeNull()
    await user.keyboard('{Escape}')

    await user.click(videoTrigger)
    await user.click(
      await screen.findByRole('option', {
        name: 'Moxing',
      })
    )

    expect(assetTrigger.textContent).toContain('Moxing Asset Library')
  })

  test('labels the URL-only protocol and selects no asset library', async () => {
    const user = userEvent.setup()
    render(<SeedanceProtocolFieldsHarness />)

    const videoTrigger = screen.getByRole('combobox', {
      name: 'Seedance Video Protocol',
    })
    const assetTrigger = screen.getByRole('combobox', {
      name: 'Seedance Asset Protocol',
    })

    await user.click(videoTrigger)
    await user.click(
      await screen.findByRole('option', {
        name: 'URL Media Arrays (URL Only, No Asset Library)',
      })
    )

    expect(videoTrigger.textContent).toContain(
      'URL Media Arrays (URL Only, No Asset Library)'
    )
    expect(assetTrigger.textContent).toContain('No Asset Protocol')
  })
})
