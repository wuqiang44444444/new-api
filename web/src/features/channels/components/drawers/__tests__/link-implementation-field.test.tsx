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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { FormProvider, useForm, useWatch } from 'react-hook-form'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { beforeAll, describe, expect, it, vi } from 'vitest'

import {
  CHANNEL_FORM_DEFAULT_VALUES,
  type ChannelFormValues,
} from '../../../lib/channel-form'
import type { LinkImplementation } from '../../../types'
import { LinkImplementationField } from '../link-implementation-field'

const implementation = {
  id: 'reactive-plan',
  version: 'v1',
  content_hash: 'sha256:test',
  provider: 'Provider',
  plan_name: 'Seedance 2.0 Overseas with Managed Assets',
  contract_id: 'contract',
  public_skus: ['video-sku'],
  channel_type: 54,
  required_video_profile: 'third_party_relay',
  required_asset_profile: 'relay_assets',
  required_sku_create_paths: [
    { public_sku: 'video-sku', create_path: '/v1/video/create' },
  ],
  execution_bindings: [
    {
      route_family: 'modelark_video',
      action: 'create',
      profile: 'third_party_relay',
      provider_model: 'provider-video-v1',
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

const implementationV2 = {
  ...implementation,
  version: 'v2',
  content_hash: 'sha256:test-v2',
  plan_name: 'Seedance 2.0 Overseas with Managed Assets Next',
} satisfies LinkImplementation

vi.mock('../../../api', () => ({
  getLinkImplementations: async () => ({
    success: true,
    data: [implementation, implementationV2],
  }),
  getLinkModelPublications: async () => ({ success: true, data: [] }),
  rebindLinkModelPublication: vi.fn(),
}))

beforeAll(async () => {
  await i18next.use(initReactI18next).init({
    lng: 'en',
    resources: { en: { translation: {} } },
  })
})

type ProjectionHarnessProps = {
  selectPlanByDefault?: boolean
  initialModels?: string
  initialModelMapping?: string
}

function ProjectionHarness(props: ProjectionHarnessProps) {
  const selectPlanByDefault = props.selectPlanByDefault !== false
  const form = useForm<ChannelFormValues>({
    defaultValues: {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      models: props.initialModels || '',
      model_mapping: props.initialModelMapping || '',
      link_implementation_id: selectPlanByDefault ? implementation.id : '',
      link_implementation_version: selectPlanByDefault
        ? implementation.version
        : '',
    },
  })
  const createPath = useWatch({
    control: form.control,
    name: 'video_upstream_create_path',
  })
  const assetMinURLTTLSeconds = useWatch({
    control: form.control,
    name: 'asset_min_url_ttl_seconds',
  })
  const selectedVersion = useWatch({
    control: form.control,
    name: 'link_implementation_version',
  })

  return (
    <FormProvider {...form}>
      <input aria-label='Models' {...form.register('models')} />
      <input aria-label='Model mapping' {...form.register('model_mapping')} />
      <output aria-label='Create path'>{createPath}</output>
      <output aria-label='Minimum asset URL TTL'>
        {assetMinURLTTLSeconds}
      </output>
      <output aria-label='Selected plan version'>{selectedVersion}</output>
      <LinkImplementationField
        control={form.control}
        channelType={54}
        canRebind
      />
    </FormProvider>
  )
}

describe('Link access plan field', () => {
  it('shows long plan names in a wider wrapping menu', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18next}>
          <ProjectionHarness />
        </I18nextProvider>
      </QueryClientProvider>
    )

    const user = userEvent.setup()
    await user.click(await screen.findByRole('combobox'))

    const listbox = await screen.findByRole('listbox')
    const popup = listbox.closest('[data-slot="select-content"]')
    const planOption = screen.getByRole('option', {
      name: 'Provider · Seedance 2.0 Overseas with Managed Assets',
    })
    expect(popup?.classList.contains('w-[460px]')).toBe(true)
    expect(popup?.classList.contains('max-w-[calc(100vw-2rem)]')).toBe(true)
    expect(
      planOption.classList.contains(
        '[&_[data-slot=select-item-text]]:whitespace-normal'
      )
    ).toBe(true)

    queryClient.clear()
  })

  it('selects exactly one version when plans share an implementation ID', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18next}>
          <ProjectionHarness />
        </I18nextProvider>
      </QueryClientProvider>
    )

    const user = userEvent.setup()
    await user.click(await screen.findByRole('combobox'))

    const v1Option = screen.getByRole('option', {
      name: 'Provider · Seedance 2.0 Overseas with Managed Assets',
    })
    const v2Option = screen.getByRole('option', {
      name: 'Provider · Seedance 2.0 Overseas with Managed Assets Next',
    })
    expect(v1Option.getAttribute('aria-selected')).toBe('true')
    expect(v2Option.getAttribute('aria-selected')).toBe('false')

    await user.click(v2Option)
    expect(screen.getByLabelText('Selected plan version').textContent).toBe(
      'v2'
    )

    queryClient.clear()
  })

  it('fills registered provider models without creating model mapping on a new video plan', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18next}>
          <ProjectionHarness selectPlanByDefault={false} />
        </I18nextProvider>
      </QueryClientProvider>
    )

    const user = userEvent.setup()
    await user.click(await screen.findByRole('combobox'))
    await user.click(
      screen.getByRole('option', {
        name: 'Provider · Seedance 2.0 Overseas with Managed Assets',
      })
    )

    await waitFor(() => {
      expect((screen.getByLabelText('Models') as HTMLInputElement).value).toBe(
        'provider-video-v1'
      )
    })
    expect(
      (screen.getByLabelText('Model mapping') as HTMLInputElement).value
    ).toBe('')
    expect(screen.queryByText('Supported Link SKUs')).toBeNull()
    expect(screen.queryByText(/video-sku/)).toBeNull()

    queryClient.clear()
  })

  it('preserves existing customer models and mapping when a video plan is selected, switched, or cleared', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18next}>
          <ProjectionHarness
            selectPlanByDefault={false}
            initialModels='customer-video'
            initialModelMapping='{"customer-video":"provider-video-v1"}'
          />
        </I18nextProvider>
      </QueryClientProvider>
    )

    const user = userEvent.setup()
    await user.click(await screen.findByRole('combobox'))
    await user.click(
      screen.getByRole('option', {
        name: 'Provider · Seedance 2.0 Overseas with Managed Assets',
      })
    )

    await user.click(screen.getByRole('combobox'))
    await user.click(
      screen.getByRole('option', {
        name: 'Provider · Seedance 2.0 Overseas with Managed Assets Next',
      })
    )
    await user.click(screen.getByRole('combobox'))
    await user.click(
      screen.getByRole('option', { name: 'No Link access plan' })
    )

    expect((screen.getByLabelText('Models') as HTMLInputElement).value).toBe(
      'customer-video'
    )
    expect(
      (screen.getByLabelText('Model mapping') as HTMLInputElement).value
    ).toBe('{"customer-video":"provider-video-v1"}')

    queryClient.clear()
  })

  it('reprojects SKU-specific fields after models are entered plan-first', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18next}>
          <ProjectionHarness />
        </I18nextProvider>
      </QueryClientProvider>
    )

    expect(screen.getByLabelText('Create path').textContent).toBe('')
    await waitFor(() => {
      expect(screen.getByLabelText('Minimum asset URL TTL').textContent).toBe(
        '3600'
      )
    })
    fireEvent.change(screen.getByLabelText('Models'), {
      target: { value: 'customer-video-v1' },
    })
    fireEvent.change(screen.getByLabelText('Model mapping'), {
      target: {
        value: '{"customer-video-v1":"provider-video-v1"}',
      },
    })

    await waitFor(() => {
      expect(screen.getByLabelText('Create path').textContent).toBe(
        '/v1/video/create'
      )
    })

    fireEvent.change(screen.getByLabelText('Models'), {
      target: { value: '' },
    })
    await waitFor(() => {
      expect(screen.getByLabelText('Create path').textContent).toBe('')
    })

    queryClient.clear()
  })
})
