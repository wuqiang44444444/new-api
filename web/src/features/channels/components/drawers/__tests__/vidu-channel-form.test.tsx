import { zodResolver } from '@hookform/resolvers/zod'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useForm } from 'react-hook-form'
import { describe, expect, test, vi } from 'vitest'

import { Form } from '@/components/ui/form'

import { publishedSeedanceConfiguration } from '../../../lib/__tests__/seedance-plugin-fixture'
import {
  channelFormSchema,
  transformChannelToFormDefaults,
  transformFormDataToCreatePayload,
  transformFormDataToUpdatePayload,
  type ChannelFormValues,
} from '../../../lib/channel-form'
import { channelSchema, type Channel } from '../../../types'
import { SeedanceConfiguredProtocolFields } from '../seedance-configured-protocol-fields'

vi.mock('../../../lib/seedance-plugin-configuration', () => ({
  getSeedancePluginConfiguration: async () => ({
    version: 'fixture-version',
    configuration: publishedSeedanceConfiguration,
  }),
}))

function Harness({
  channel,
  save,
}: {
  channel: Channel
  save: (values: ChannelFormValues) => void
}) {
  const form = useForm<ChannelFormValues>({
    resolver: zodResolver(channelFormSchema),
    defaultValues: transformChannelToFormDefaults(channel),
  })
  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(save)}>
        <SeedanceConfiguredProtocolFields
          control={form.control}
          sensitiveLocked={false}
        />
        <button type='submit'>Save</button>
      </form>
    </Form>
  )
}

function viduChannel(region: 'cn' | 'global'): Channel {
  const models = ['2-5', '2-0', '2-0-fast', '2-0-mini'].map(
    (model) => `seedance-${model}-vidu-${region}`
  )
  const providerModels =
    region === 'cn'
      ? [
          'viduq3.1-drama-std',
          'viduq3-drama-std',
          'viduq3-drama-fast',
          'viduq3-drama-mini',
        ]
      : [
          'viduq3.1-drama-ab-std',
          'viduq3-drama-ab-std',
          'viduq3-drama-ab-fast',
          'viduq3-drama-ab-mini',
        ]
  return channelSchema.parse({
    id: 1,
    type: 62,
    key: '',
    status: 1,
    name: `Vidu ${region}`,
    created_time: 0,
    test_time: 0,
    response_time: 0,
    balance_updated_time: 0,
    base_url: `https://api.vidu.${region === 'cn' ? 'cn' : 'com'}/ent`,
    models: models.join(','),
    model_mapping: JSON.stringify(
      Object.fromEntries(
        models.map((model, index) => [model, providerModels[index]])
      )
    ),
    settings: JSON.stringify({
      video_upstream_protocol: 'vidu_modelark_v3',
      asset_upstream_protocol: 'none',
    }),
  })
}

describe('Vidu channel administration', () => {
  test.each(['cn', 'global'] as const)(
    'loads and saves the %s channel without changing credentials, mapping or connection',
    async (region) => {
      const channel = viduChannel(region)
      const save = vi.fn()
      const client = new QueryClient({
        defaultOptions: { queries: { retry: false } },
      })
      render(
        <QueryClientProvider client={client}>
          <Harness channel={channel} save={save} />
        </QueryClientProvider>
      )
      const selector = await screen.findByRole('combobox', {
        name: 'Seedance Video Protocol',
      })
      expect(selector).toHaveTextContent('Vidu Drama ModelArk V3')
      await userEvent.click(screen.getByRole('button', { name: 'Save' }))
      await waitFor(() => expect(save).toHaveBeenCalledTimes(1))
      const values = save.mock.calls[0][0] as ChannelFormValues
      const update = transformFormDataToUpdatePayload(values, channel.id)
      expect(update).toMatchObject({
        id: channel.id,
        base_url: channel.base_url,
        models: channel.models,
        model_mapping: channel.model_mapping,
        seedance_plugin_version: 'fixture-version',
      })
      expect(update).not.toHaveProperty('key')
      expect(update).not.toHaveProperty('asset_credential')
      expect(JSON.parse(update.settings ?? '{}')).toMatchObject({
        video_upstream_protocol: 'vidu_modelark_v3',
        asset_upstream_protocol: 'none',
      })
      const create = transformFormDataToCreatePayload({
        ...values,
        key: 'fixture-only',
      })
      expect(create.channel.base_url).toBe(channel.base_url)
      expect(create.channel.models).toBe(channel.models)
      expect(create.channel.model_mapping).toBe(channel.model_mapping)
      expect(
        JSON.parse(create.channel.settings ?? '{}').video_upstream_protocol
      ).toBe('vidu_modelark_v3')
    }
  )

  test('switching to Vidu clears incompatible material configuration before saving', async () => {
    const channel = viduChannel('cn')
    channel.settings = JSON.stringify({
      video_upstream_protocol: 'modelark_v3_volcengine',
      asset_upstream_protocol: 'volcengine_assets_action_v2024_01_01',
      asset_region: 'cn-beijing',
      asset_provider_project: 'legacy-project',
      asset_min_url_ttl_seconds: 3600,
    })
    const save = vi.fn()
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={client}>
        <Harness channel={channel} save={save} />
      </QueryClientProvider>
    )
    await userEvent.click(
      await screen.findByRole('combobox', { name: 'Seedance Video Protocol' })
    )
    await userEvent.click(
      await screen.findByRole('option', { name: 'Vidu Drama ModelArk V3' })
    )
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(save).toHaveBeenCalledTimes(1))
    const payload = transformFormDataToUpdatePayload(
      save.mock.calls[0][0],
      channel.id
    )
    const settings = JSON.parse(payload.settings ?? '{}')
    expect(settings).toMatchObject({
      video_upstream_protocol: 'vidu_modelark_v3',
      asset_upstream_protocol: 'none',
      asset_min_url_ttl_seconds: 0,
    })
    expect(settings).not.toHaveProperty('asset_region')
    expect(settings).not.toHaveProperty('asset_provider_project')
    expect(payload).not.toHaveProperty('asset_credential')
  })
})
