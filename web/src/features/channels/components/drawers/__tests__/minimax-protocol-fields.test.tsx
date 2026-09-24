import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useForm, useWatch } from 'react-hook-form'
import { describe, expect, test, vi } from 'vitest'

import { Button } from '@/components/ui/button'
import { Form } from '@/components/ui/form'
import { api } from '@/lib/api'

import {
  CHANNEL_FORM_DEFAULT_VALUES,
  type ChannelFormValues,
} from '../../../lib/channel-form'
import { resetMinimaxConnectionDraft } from '../../../lib/minimax-management'
import { MinimaxProtocolFields } from '../minimax-protocol-fields'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn() } }))

function Harness() {
  const form = useForm<ChannelFormValues>({
    defaultValues: { ...CHANNEL_FORM_DEFAULT_VALUES, type: 64 },
  })
  const version = useWatch({
    control: form.control,
    name: 'minimax_plugin_version',
  })
  const type = useWatch({ control: form.control, name: 'type' })
  return (
    <Form {...form}>
      {type === 64 && <MinimaxProtocolFields />}
      <output aria-label='Submitted declaration'>{version}</output>
      <Button
        onClick={() => form.reset({ ...CHANNEL_FORM_DEFAULT_VALUES, type: 64 })}
      >
        Reload channel
      </Button>
      <Button onClick={() => resetMinimaxConnectionDraft(form, 35, true)}>
        Use native API
      </Button>
    </Form>
  )
}

const declaration = {
  version: '1.2.0',
  configuration: {
    videos: [
      {
        protocol: 'jdcloud_video_task_v1',
        label: 'JD Cloud Video Task V1',
        models: ['MiniMax-H3'],
        modelMetadata: {
          'MiniMax-H3': {
            minDuration: 4,
            maxDuration: 15,
            resolutions: ['768p', '2k'],
            maxImages: 9,
          },
        },
        assetProtocols: ['none'],
        defaultAssetProtocol: 'none',
      },
    ],
    assets: [],
  },
}

describe('MiniMax declaration display', () => {
  test('a late declaration response cannot populate a different access method', async () => {
    let resolveRequest!: (value: {
      data: { success: boolean; data: typeof declaration }
    }) => void
    const response = new Promise<{
      data: { success: boolean; data: typeof declaration }
    }>((resolve) => {
      resolveRequest = resolve
    })
    vi.mocked(api.get).mockReturnValue(response)
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={client}>
        <Harness />
      </QueryClientProvider>
    )
    await waitFor(() => expect(api.get).toHaveBeenCalled())
    await userEvent.click(
      screen.getByRole('button', { name: 'Use native API' })
    )
    await act(async () => {
      resolveRequest({ data: { success: true, data: declaration } })
      await response
    })
    expect(screen.getByLabelText('Submitted declaration').textContent).toBe('')
    expect(screen.queryByText('MiniMax-H3')).toBeNull()
    client.clear()
  })
  test('shows declaration limits and restores its version after the channel form resets', async () => {
    vi.mocked(api.get).mockResolvedValue({
      data: { success: true, data: declaration },
    })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={client}>
        <Harness />
      </QueryClientProvider>
    )
    expect(await screen.findByText('MiniMax-H3')).toBeTruthy()
    expect(screen.getByText('768p, 2k')).toBeTruthy()
    expect(screen.queryByText(/First-phase open set/)).toBeNull()
    await waitFor(() =>
      expect(screen.getByLabelText('Submitted declaration').textContent).toBe(
        '1.2.0'
      )
    )
    await userEvent.click(
      screen.getByRole('button', { name: 'Reload channel' })
    )
    await waitFor(() =>
      expect(screen.getByLabelText('Submitted declaration').textContent).toBe(
        '1.2.0'
      )
    )
    client.clear()
  })

  test('does not pin a version when the declaration is missing', async () => {
    vi.mocked(api.get).mockResolvedValue({
      data: { success: true, data: { version: '1.2.0', configuration: null } },
    })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={client}>
        <Harness />
      </QueryClientProvider>
    )
    await screen.findByRole('button', { name: /retry/i })
    expect(screen.getByLabelText('Submitted declaration').textContent).toBe('')
    client.clear()
  })
})
