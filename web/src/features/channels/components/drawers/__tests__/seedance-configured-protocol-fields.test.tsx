import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useForm, useWatch } from 'react-hook-form'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { Form } from '@/components/ui/form'

import {
  CHANNEL_FORM_DEFAULT_VALUES,
  type ChannelFormValues,
} from '../../../lib/channel-form'
import type { SeedanceConfigurationSnapshot } from '../../../lib/seedance-plugin-configuration'
import { SeedanceConfiguredProtocolFields } from '../seedance-configured-protocol-fields'

const { readConfiguration } = vi.hoisted(() => ({ readConfiguration: vi.fn() }))
vi.mock('../../../lib/seedance-plugin-configuration', () => ({
  getSeedancePluginConfiguration: readConfiguration,
}))
vi.mock('../seedance-protocol-fields', () => ({
  SeedanceProtocolFields: ({
    configuration,
  }: {
    configuration?: SeedanceConfigurationSnapshot['configuration']
  }) => (
    <output data-testid='definition'>
      {configuration?.videos[0]?.label ?? 'legacy'}
    </output>
  ),
}))

function snapshot(version: string): SeedanceConfigurationSnapshot {
  return {
    version,
    configuration: {
      videos: [
        {
          protocol: 'feicai_videos_v1',
          label: version,
          models: ['provider-model'],
          assetProtocols: ['none'],
          defaultAssetProtocol: 'none',
        },
      ],
      assets: [
        {
          protocol: 'none',
          label: 'None',
          groupPolicy: 'none',
          credential: 'none',
        },
      ],
    },
  }
}

function Harness() {
  const form = useForm<ChannelFormValues>({
    defaultValues: {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      type: 62,
      video_upstream_protocol: 'feicai_videos_v1',
    },
  })
  const version = useWatch({
    control: form.control,
    name: 'seedance_plugin_version',
  })
  const configuration = useWatch({
    control: form.control,
    name: 'seedance_plugin_configuration',
  })
  return (
    <Form {...form}>
      <SeedanceConfiguredProtocolFields
        control={form.control}
        sensitiveLocked={false}
      />
      <output data-testid='submitted-version'>{version}</output>
      <output data-testid='validation-definition'>
        {configuration?.videos[0]?.label}
      </output>
      <button
        type='button'
        onClick={() => form.reset({ ...CHANNEL_FORM_DEFAULT_VALUES, type: 62 })}
      >
        Reset form
      </button>
    </Form>
  )
}

beforeEach(() => {
  readConfiguration.mockReset()
})

describe('Seedance configuration snapshot', () => {
  test('keeps one declaration for display, validation and save across refresh and form reset', async () => {
    readConfiguration.mockResolvedValue(snapshot('2.0.0'))
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={client}>
        <Harness />
      </QueryClientProvider>
    )
    await waitFor(() =>
      expect(screen.getByTestId('submitted-version')).toHaveTextContent('2.0.0')
    )
    expect(screen.getByTestId('definition')).toHaveTextContent('2.0.0')
    expect(screen.getByTestId('validation-definition')).toHaveTextContent(
      '2.0.0'
    )
    act(() =>
      client.setQueriesData(
        { queryKey: ['seedance-channel-configuration'] },
        snapshot('2.1.0')
      )
    )
    await userEvent.click(screen.getByRole('button', { name: 'Reset form' }))
    await waitFor(() =>
      expect(screen.getByTestId('submitted-version')).toHaveTextContent('2.0.0')
    )
    expect(screen.getByTestId('definition')).toHaveTextContent('2.0.0')
    expect(screen.getByTestId('validation-definition')).toHaveTextContent(
      '2.0.0'
    )
    expect(readConfiguration).toHaveBeenCalledTimes(1)
  })

  test('does not render a legacy definition when reading the active declaration fails', async () => {
    readConfiguration.mockRejectedValue(new Error('unavailable'))
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={client}>
        <Harness />
      </QueryClientProvider>
    )
    await waitFor(() =>
      expect(screen.getByRole('button', { name: /retry/i })).toBeInTheDocument()
    )
    expect(screen.queryByTestId('definition')).not.toBeInTheDocument()
    expect(screen.getByTestId('submitted-version')).toBeEmptyDOMElement()
  })
})
