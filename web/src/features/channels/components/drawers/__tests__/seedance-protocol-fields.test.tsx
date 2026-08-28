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
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { useForm } from 'react-hook-form'
import { I18nextProvider } from 'react-i18next'
import { beforeAll, describe, expect, test } from 'vitest'

import { Form } from '@/components/ui/form'

import type { AssetTenantBoundaryChange } from '../../../lib/asset-tenant-boundary'
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
          'TokenSave Media Task V1': 'TokenSave Media Task V1',
          'Moxing Media Task V1': 'Moxing Media Task V1',
          'Moxing ModelArk Media Task V1': 'Moxing ModelArk Media Task V1',
          'Ark Media V1': 'Ark Proxy Video API',
          'Feicai Videos V1 (URL Only, No Asset Library)':
            'Feicai Videos V1 (URL Only, No Asset Library)',
          'FunCloud Seedance': 'FunCloud',
          'FunCloud Material Library': 'FunCloud Material Library',
          'Volcengine Official Assets': 'Volcengine Official Assets',
          'BytePlus Official Assets': 'BytePlus Official Assets',
          'No Asset Protocol': 'No Asset Protocol',
          'TokenSave Asset Library V1': 'TokenSave Asset Library V1',
          'Moxing JoyCreator Asset Library V1':
            'Moxing JoyCreator Asset Library V1',
          'Moxing Volcengine Asset Library V1':
            'Moxing Volcengine Asset Library V1',
          'Ark Assets V1': 'Ark Proxy Asset Library',
        },
      },
    },
  })
})

function SeedanceProtocolFieldsHarness(
  props: {
    models?: string
    modelMapping?: string
    boundaryChanges?: AssetTenantBoundaryChange[]
  } = {}
) {
  const form = useForm<ChannelFormValues>({
    defaultValues: {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      type: 61,
      models: props.models || '',
      model_mapping: props.modelMapping || '',
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
          boundaryChanges={props.boundaryChanges}
        />
      </Form>
    </I18nextProvider>
  )
}

describe('Seedance protocol fields', () => {
  test('explains the channel asset boundary and lists the sharing models', () => {
    render(<SeedanceProtocolFieldsHarness models='seedance-a, seedance-b' />)

    expect(
      screen.getByText('Asset library boundary: this channel')
    ).toBeTruthy()
    expect(
      screen.getByText(
        'Models sharing this asset library: seedance-a, seedance-b'
      )
    ).toBeTruthy()
  })

  test('shows the exact in-form warning when a boundary field changes', () => {
    render(
      <SeedanceProtocolFieldsHarness
        boundaryChanges={[
          {
            field: 'asset_provider_project',
            previous: 'default',
            next: 'lumen-test',
          },
        ]}
      />
    )

    expect(
      screen.getByText('This save will replace the asset tenant')
    ).toBeTruthy()
    expect(screen.getByText(/default.*lumen-test/)).toBeTruthy()
  })

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
        name: 'Moxing Media Task V1',
      })
    )

    expect(assetTrigger.textContent).toContain(
      'Moxing JoyCreator Asset Library V1'
    )
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
        name: 'Feicai Videos V1 (URL Only, No Asset Library)',
      })
    )

    expect(videoTrigger.textContent).toContain(
      'Feicai Videos V1 (URL Only, No Asset Library)'
    )
    expect(assetTrigger.textContent).toContain('No Asset Protocol')
  })

  test('selects no asset library for a customer model mapped to FunCloud 2.5', async () => {
    const user = userEvent.setup()
    render(
      <SeedanceProtocolFieldsHarness
        models='customer-next'
        modelMapping='{"customer-next":"seedance-2-5"}'
      />
    )

    const videoTrigger = screen.getByRole('combobox', {
      name: 'Seedance Video Protocol',
    })
    const assetTrigger = screen.getByRole('combobox', {
      name: 'Seedance Asset Protocol',
    })
    await user.click(videoTrigger)
    await user.click(await screen.findByRole('option', { name: 'FunCloud' }))

    expect(assetTrigger.textContent).toContain('No Asset Protocol')
    await user.click(assetTrigger)
    const listbox = await screen.findByRole('listbox')
    expect(
      within(listbox).queryByRole('option', {
        name: 'FunCloud Material Library',
      })
    ).toBeNull()
  })
})
