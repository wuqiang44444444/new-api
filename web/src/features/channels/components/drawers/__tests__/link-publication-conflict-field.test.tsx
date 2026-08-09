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
import { render, screen, within } from '@testing-library/react'
import i18next from 'i18next'
import { useForm } from 'react-hook-form'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { beforeAll, describe, expect, it, vi } from 'vitest'

import {
  CHANNEL_FORM_DEFAULT_VALUES,
  type ChannelFormValues,
} from '../../../lib/channel-form'
import type { LinkImplementation } from '../../../types'
import { LinkPublicationConflictField } from '../link-publication-conflict-field'

const implementation = {
  id: 'reactive-plan',
  version: 'v1',
  content_hash: 'sha256:test',
  provider: 'Provider',
  plan_name: 'Video Plan',
  contract_id: 'contract',
  public_skus: ['proposed-video-sku'],
  channel_type: 54,
  required_video_profile: 'third_party_relay',
  execution_bindings: [
    {
      route_family: 'modelark_video',
      action: 'create',
      profile: 'third_party_relay',
      provider_model: 'provider-video-v1',
      link_sku: 'proposed-video-sku',
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
  getLinkModelPublications: async () => ({
    success: true,
    data: [
      {
        id: 1,
        contract_namespace: 'link',
        route_family: 'modelark_video',
        customer_model: 'customer-video',
        link_sku: 'published-video-sku',
        publication_version: 2,
        source_channel_id: 56,
        change_reason: '',
        created_at: 1,
        updated_at: 1,
        currently_fulfillable: true,
        routing_conflict: true,
      },
    ],
  }),
  rebindLinkModelPublication: vi.fn(),
}))

beforeAll(async () => {
  await i18next.use(initReactI18next).init({
    lng: 'en',
    resources: { en: { translation: {} } },
  })
})

function ConflictHarness() {
  const form = useForm<ChannelFormValues>({
    defaultValues: {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      models: 'customer-video',
      model_mapping: '{"customer-video":"provider-video-v1"}',
      link_implementation_id: implementation.id,
      link_implementation_version: implementation.version,
    },
  })

  return <LinkPublicationConflictField control={form.control} canRebind />
}

describe('Link publication conflict field', () => {
  it('keeps an explicit publication rebind in the model mapping area', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18next}>
          <ConflictHarness />
        </I18nextProvider>
      </QueryClientProvider>
    )

    const conflict = await screen.findByRole('alert')
    expect(
      within(conflict).getByText('customer-video → provider-video-v1')
    ).not.toBeNull()
    expect(
      within(conflict).getByRole('button', { name: 'Rebind' })
    ).not.toBeNull()

    queryClient.clear()
  })
})
