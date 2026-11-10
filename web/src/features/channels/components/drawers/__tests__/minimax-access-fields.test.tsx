import { zodResolver } from '@hookform/resolvers/zod'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useForm } from 'react-hook-form'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { Button } from '@/components/ui/button'
import { Form } from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { api } from '@/lib/api'

import {
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformFormDataToCreatePayload,
  type ChannelFormValues,
} from '../../../lib/channel-form'
import { selectChannelManagementType } from '../../../lib/minimax-management'
import { MinimaxAccessFields } from '../minimax-access-fields'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn() } }))

const queryClient = new QueryClient()
afterEach(() => {
  queryClient.clear()
  vi.resetAllMocks()
})

function renderAccess(
  props: Parameters<typeof Harness>[0],
  supportsNexhina = false,
  pluginEnabled = true
) {
  const protocols = ['jdcloud_video_task_v1']
  if (supportsNexhina) protocols.push('nexhina_hailuo_v2')
  vi.mocked(api.get).mockResolvedValue({
    data: {
      success: true,
      data: {
        version: supportsNexhina ? '1.5.0' : '1.2.0',
        enabled: pluginEnabled,
        configuration: {
          videos: protocols.map((protocol) => ({
            protocol,
            models: ['MiniMax-H3'],
            modelMetadata: {},
          })),
        },
      },
    },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <Harness {...props} />
    </QueryClientProvider>
  )
}

function Harness(props: {
  save: (payload: ReturnType<typeof transformFormDataToCreatePayload>) => void
  editing?: boolean
  disabled?: boolean
  type?: number
}) {
  const form = useForm<ChannelFormValues>({
    resolver: zodResolver(channelFormSchema),
    defaultValues: {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'MiniMax channel',
      type: props.type ?? 35,
      minimax_access_selected: props.editing ? undefined : false,
      models: 'customer',
      key: 'fixture-key',
      base_url: 'https://fixture.invalid',
      model_mapping: '{"customer":"MiniMax-H3"}',
      settings: '{"video_upstream_protocol":"jdcloud_video_task_v1"}',
      video_upstream_protocol: 'jdcloud_video_task_v1',
    },
  })
  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit((data) =>
          props.save(transformFormDataToCreatePayload(data))
        )}
      >
        <MinimaxAccessFields
          editing={props.editing ?? false}
          disabled={props.disabled ?? false}
        />
        <Input aria-label='Credential' {...form.register('key')} />
        <Input aria-label='Endpoint' {...form.register('base_url')} />
        <Input aria-label='Models' {...form.register('models')} />
        <Input aria-label='Mapping' {...form.register('model_mapping')} />
        <Input
          aria-label='Declaration'
          {...form.register('minimax_plugin_version')}
        />
        <Button
          type='button'
          onClick={() =>
            selectChannelManagementType(
              form,
              1,
              props.editing ? (props.type ?? 35) : undefined
            )
          }
        >
          Other provider
        </Button>
        <Button type='submit'>Save</Button>
        <Button
          type='button'
          onClick={() =>
            selectChannelManagementType(
              form,
              35,
              props.editing ? (props.type ?? 35) : undefined
            )
          }
        >
          Native MiniMax
        </Button>
      </form>
    </Form>
  )
}

describe('MiniMax access choice', () => {
  test('shows the activation and pricing requirements while retaining the disabled plugin declaration', async () => {
    renderAccess({ save: vi.fn(), editing: true, type: 69 }, false, false)
    expect(
      await screen.findByText(
        'The MiniMax plugin is disabled. Enable it before enabling this channel.'
      )
    ).toBeVisible()
    expect(
      screen.getByText(
        'Set a tiered billing expression for each customer model before sending requests.'
      )
    ).toBeVisible()
    expect(screen.getByText('Declared video capabilities')).toBeVisible()
  })

  test('another native provider can switch to MiniMax and back without losing its connection draft', async () => {
    const user = userEvent.setup()
    const save = vi.fn()
    renderAccess({ save, editing: true, type: 1 })
    await user.click(screen.getByRole('button', { name: 'Native MiniMax' }))
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(save).toHaveBeenCalledTimes(1))
    expect(save.mock.calls[0][0].channel.type).toBe(35)
    expect(save.mock.calls[0][0].channel.key).toBe('fixture-key')
    await user.click(screen.getByRole('button', { name: 'Other provider' }))
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(save).toHaveBeenCalledTimes(2))
    expect(save.mock.calls[1][0].channel.type).toBe(1)
    expect(save.mock.calls[1][0].channel.key).toBe('fixture-key')
  })
  test('access choices are available through the keyboard', async () => {
    const user = userEvent.setup()
    renderAccess({ save: vi.fn() })
    const choice = screen.getByRole('combobox', { name: 'Access method' })
    await user.tab()
    await user.keyboard('{ArrowDown}')
    await screen.findByRole('option', { name: 'Native API' })
    await user.keyboard('{Enter}')
    await waitFor(() =>
      expect((choice as HTMLInputElement).value).toBe('Native API')
    )
  })
  test('requires an explicit access method before saving a new connection', async () => {
    const save = vi.fn()
    renderAccess({ save })
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(
        screen
          .getByRole('combobox', { name: 'Access method' })
          .getAttribute('aria-invalid')
      ).toBe('true')
    )
    expect(save).not.toHaveBeenCalled()
  })

  test.each([
    ['Native API', 35],
    ['JD Cloud · Standard video', 69],
  ] as const)(
    'selecting %s clears private fields and submits its real type',
    async (label, type) => {
      const user = userEvent.setup()
      const save = vi.fn()
      renderAccess({ save })
      const choice = screen.getByRole('combobox', { name: 'Access method' })
      await user.click(choice)
      const option = await screen.findByRole('option', { name: label })
      await waitFor(() =>
        expect(option).not.toHaveAttribute('aria-disabled', 'true')
      )
      await user.click(option)
      for (const field of ['Credential', 'Endpoint', 'Models', 'Mapping']) {
        expect(
          (screen.getByRole('textbox', { name: field }) as HTMLInputElement)
            .value
        ).toBe('')
      }
      await user.type(
        screen.getByRole('textbox', { name: 'Credential' }),
        'new-fixture-key'
      )
      await user.type(
        screen.getByRole('textbox', { name: 'Models' }),
        'customer'
      )
      if (type === 69) {
        await user.type(
          screen.getByRole('textbox', { name: 'Endpoint' }),
          'http://fixture.invalid'
        )
      }
      await user.click(screen.getByRole('button', { name: 'Save' }))
      await waitFor(() => expect(save).toHaveBeenCalledTimes(1))
      const payload = save.mock.calls[0][0]
      expect(payload.channel.type).toBe(type)
      expect(payload.channel.name).toBe('MiniMax channel')
      expect(payload.channel.minimax_access_selected).toBeUndefined()
      if (type === 69) {
        expect(
          JSON.parse(payload.channel.settings).video_upstream_protocol
        ).toBe('jdcloud_video_task_v1')
      }

      await user.click(choice)
      await user.click(
        await screen.findByRole('option', {
          name: type === 69 ? 'Native API' : 'JD Cloud · Standard video',
        })
      )
      expect(
        (
          screen.getByRole('textbox', {
            name: 'Credential',
          }) as HTMLInputElement
        ).value
      ).toBe('')
      expect(
        (
          screen.getByRole('textbox', {
            name: 'Declaration',
          }) as HTMLInputElement
        ).value
      ).toBe(type === 69 ? '' : '1.2.0')
    }
  )

  test('saved native MiniMax keeps its read-only access method', () => {
    renderAccess({ save: vi.fn(), editing: true, type: 35 })
    expect(screen.queryByRole('combobox', { name: 'Access method' })).toBeNull()
    expect(screen.getByText('Native API')).toBeVisible()
  })

  test('an older declaration marks Nexhina unavailable and keeps it unselected', async () => {
    const user = userEvent.setup()
    renderAccess({ save: vi.fn() })
    await user.click(screen.getByRole('combobox', { name: 'Access method' }))
    const option = await screen.findByRole('option', { name: /Nexhina/ })
    expect(option).toHaveAttribute('aria-disabled', 'true')
    expect(option).toHaveTextContent('Unavailable in plugin version 1.2.0')
    expect(screen.getByRole('combobox', { name: 'Access method' })).toHaveValue(
      ''
    )
  })

  test('selecting Nexhina prefills an editable endpoint and saves the declared protocol', async () => {
    const user = userEvent.setup()
    const save = vi.fn()
    renderAccess({ save }, true)
    await user.click(screen.getByRole('combobox', { name: 'Access method' }))
    const option = await screen.findByRole('option', { name: /Nexhina/ })
    await waitFor(() =>
      expect(option).not.toHaveAttribute('aria-disabled', 'true')
    )
    await user.click(option)
    const endpoint = screen.getByRole('textbox', { name: 'Endpoint' })
    expect(endpoint).toHaveValue('https://token.nexhina.cn')
    await user.clear(endpoint)
    await user.type(endpoint, 'https://proxy.invalid/prefix')
    await user.type(
      screen.getByRole('textbox', { name: 'Credential' }),
      'new-fixture-key'
    )
    await user.type(screen.getByRole('textbox', { name: 'Models' }), 'customer')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(save).toHaveBeenCalledTimes(1))
    const channel = save.mock.calls[0][0].channel
    expect(channel.base_url).toBe('https://proxy.invalid/prefix')
    expect(channel.minimax_plugin_version).toBe('1.5.0')
    expect(JSON.parse(channel.settings).video_upstream_protocol).toBe(
      'nexhina_hailuo_v2'
    )
  })

  test('a locked form cannot select an access method', () => {
    renderAccess({ save: vi.fn(), disabled: true })
    expect(
      (
        screen.getByRole('combobox', {
          name: 'Access method',
        }) as HTMLButtonElement
      ).disabled
    ).toBe(true)
  })
})
