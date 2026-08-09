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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { FormProvider, useForm } from 'react-hook-form'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { beforeAll, describe, expect, it, vi } from 'vitest'

import {
  CHANNEL_FORM_DEFAULT_VALUES,
  type ChannelFormValues,
} from '../../../lib/channel-form'
import type { LinkImplementation } from '../../../types'
import { LinkAwareModelMappingEditor } from '../link-aware-model-mapping-editor'

const implementation = {
  id: 'reactive-plan',
  version: 'v3',
  content_hash: 'sha256:test',
  provider: 'Provider',
  plan_name: 'Seedance 2.0 Video',
  contract_id: 'contract',
  public_skus: ['video-sku'],
  channel_type: 54,
  required_video_profile: 'third_party_relay',
  required_asset_profile: 'relay_assets',
  required_sku_create_paths: [],
  execution_bindings: [
    {
      route_family: 'modelark_video',
      action: 'create',
      profile: 'third_party_relay',
      provider_model: 'provider-video-v3',
      link_sku: 'video-sku',
    },
  ],
  asset_capability: {
    supports_managed_assets: false,
    supports_mixed_media_paths: false,
  },
  task_contract: 'task',
  billing_contract: 'billing',
} satisfies LinkImplementation

vi.mock('../../../api', () => ({
  getLinkImplementations: async () => ({
    success: true,
    data: [implementation],
  }),
}))

beforeAll(async () => {
  await i18next.use(initReactI18next).init({
    lng: 'en',
    resources: { en: { translation: {} } },
  })
})

function MappingHarness() {
  const form = useForm<ChannelFormValues>({
    defaultValues: {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      link_implementation_id: implementation.id,
      link_implementation_version: implementation.version,
    },
  })

  return (
    <FormProvider {...form}>
      <LinkAwareModelMappingEditor
        control={form.control}
        channelType={54}
        value=''
        onChange={() => undefined}
        targetModelOptions={['ordinary-provider-model']}
      />
    </FormProvider>
  )
}

describe('Link-aware model mapping editor', () => {
  it('suggests the selected Link implementation models as mapping targets', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18next}>
          <MappingHarness />
        </I18nextProvider>
      </QueryClientProvider>
    )

    await userEvent.click(screen.getByRole('button', { name: 'Add Mapping' }))
    const replacementInput = screen.getByPlaceholderText(
      'gpt-3.5-turbo-0125'
    ) as HTMLInputElement
    await waitFor(() => {
      expect(
        replacementInput.list?.querySelector(
          'option[value="provider-video-v3"]'
        )
      ).not.toBeNull()
    })
    expect(
      replacementInput.list?.querySelector(
        'option[value="ordinary-provider-model"]'
      )
    ).toBeNull()

    queryClient.clear()
  })
})
