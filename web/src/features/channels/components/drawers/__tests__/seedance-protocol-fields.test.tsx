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
import { publishedSeedanceConfiguration } from '../../../lib/__tests__/seedance-plugin-fixture'
import { SeedanceProtocolFields } from '../seedance-protocol-fields'

beforeAll(async () => {
  // No translation resources: unregistered keys pass through, so option text
  // equals the declared label from the repository artifact.
  await i18next.init({ lng: 'en', fallbackLng: 'en' })
})

// Selector labels are asserted against the published repository artifact, not
// a hand-copied provider list: a declaration change that drops a protocol or
// renames a label must fail here.
function declaredVideoLabel(protocol: string): string {
  const video = publishedSeedanceConfiguration.videos.find(
    (video) => video.protocol === protocol
  )
  if (!video) throw new Error(`missing declared video protocol: ${protocol}`)
  return video.label
}

function declaredAssetLabel(protocol: string): string {
  const asset = publishedSeedanceConfiguration.assets.find(
    (asset) => asset.protocol === protocol
  )
  if (!asset) throw new Error(`missing declared asset protocol: ${protocol}`)
  return asset.label
}

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
      type: 62,
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
          configuration={publishedSeedanceConfiguration}
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

    const volcengineAssetLabel = declaredAssetLabel(
      'volcengine_assets_action_v2024_01_01'
    )
    const videoTrigger = screen.getByRole('combobox', {
      name: 'Seedance Video Protocol',
    })
    const assetTrigger = screen.getByRole('combobox', {
      name: 'Seedance Asset Protocol',
    })
    expect(videoTrigger.className).toContain('max-w-2xl')
    expect(assetTrigger.className).toContain('max-w-2xl')
    expect(assetTrigger.textContent).toContain(volcengineAssetLabel)

    await user.click(assetTrigger)
    const initialListbox = await screen.findByRole('listbox')
    const assetPopup = document.querySelector<HTMLElement>(
      '[data-slot="select-content"]'
    )
    expect(assetPopup?.className).toContain('sm:min-w-[36rem]')
    expect(
      within(initialListbox).getByRole('option', {
        name: volcengineAssetLabel,
      })
    ).toBeTruthy()
    expect(
      within(initialListbox).queryByRole('option', {
        name: declaredAssetLabel('byteplus_assets_action_v2024_01_01'),
      })
    ).toBeNull()
    await user.keyboard('{Escape}')

    await user.click(videoTrigger)
    await user.click(
      await screen.findByRole('option', {
        name: declaredVideoLabel('moxing_modelark_media_v1'),
      })
    )

    expect(assetTrigger.textContent).toContain(
      declaredAssetLabel('moxing_volc_assets_v1')
    )
  })

  test('labels the URL-only protocol and selects no asset library', async () => {
    const user = userEvent.setup()
    render(<SeedanceProtocolFieldsHarness />)

    const feicaiLabel = declaredVideoLabel('feicai_videos_v1')
    const videoTrigger = screen.getByRole('combobox', {
      name: 'Seedance Video Protocol',
    })
    const assetTrigger = screen.getByRole('combobox', {
      name: 'Seedance Asset Protocol',
    })

    await user.click(videoTrigger)
    await user.click(
      await screen.findByRole('option', { name: feicaiLabel })
    )

    expect(videoTrigger.textContent).toContain(feicaiLabel)
    expect(assetTrigger.textContent).toContain(
      declaredAssetLabel('none')
    )
  })

  test('pairs FunCloud ModelArk V3 with the FunCloud material library', async () => {
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
        name: declaredVideoLabel('funcloud_modelark_v3'),
      })
    )

    expect(assetTrigger.textContent).toContain(
      declaredAssetLabel('funcloud_material')
    )
    await user.click(assetTrigger)
    const listbox = await screen.findByRole('listbox')
    expect(
      within(listbox).getByRole('option', {
        name: declaredAssetLabel('funcloud_material'),
      })
    ).toBeTruthy()
  })
})
