import { zodResolver } from '@hookform/resolvers/zod'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useForm } from 'react-hook-form'
import { describe, expect, test, vi } from 'vitest'

import { Button } from '@/components/ui/button'
import { Form } from '@/components/ui/form'
import { Input } from '@/components/ui/input'

import {
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformFormDataToCreatePayload,
  type ChannelFormValues,
} from '../../../lib/channel-form'
import { selectChannelManagementType } from '../../../lib/minimax-management'
import { MinimaxAccessFields } from '../minimax-access-fields'

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
  test('another native provider can switch to MiniMax and back without losing its connection draft', async () => {
    const user = userEvent.setup()
    const save = vi.fn()
    render(<Harness save={save} editing type={1} />)
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
    render(<Harness save={vi.fn()} />)
    const choice = screen.getByRole('combobox', { name: 'Access method' })
    await user.tab()
    await screen.findByRole('option', { name: 'Native API' })
    await user.keyboard('{ArrowDown}{Enter}')
    await waitFor(() =>
      expect((choice as HTMLInputElement).value).toBe('Native API')
    )
  })
  test('requires an explicit access method before saving a new connection', async () => {
    const save = vi.fn()
    render(<Harness save={save} />)
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
    ['JD Cloud · Standard video', 64],
  ] as const)(
    'selecting %s clears private fields and submits its real type',
    async (label, type) => {
      const user = userEvent.setup()
      const save = vi.fn()
      render(<Harness save={save} />)
      const choice = screen.getByRole('combobox', { name: 'Access method' })
      await user.click(choice)
      await user.click(await screen.findByRole('option', { name: label }))
      for (const field of [
        'Credential',
        'Endpoint',
        'Models',
        'Mapping',
        'Declaration',
      ]) {
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
      if (type === 64) {
        await user.type(
          screen.getByRole('textbox', { name: 'Declaration' }),
          '1.2.0'
        )
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
      if (type === 64) {
        expect(
          JSON.parse(payload.channel.settings).video_upstream_protocol
        ).toBe('jdcloud_video_task_v1')
      }

      await user.click(choice)
      await user.click(
        await screen.findByRole('option', {
          name: type === 64 ? 'Native API' : 'JD Cloud · Standard video',
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
      ).toBe('')
    }
  )

  test('saved MiniMax channels show their access method without allowing a type change', async () => {
    render(<Harness save={vi.fn()} editing type={64} />)
    expect(screen.queryByRole('combobox')).toBeNull()
    expect(screen.getByDisplayValue('JD Cloud · Standard video')).toBeTruthy()
    await userEvent.click(
      screen.getByRole('button', { name: 'Other provider' })
    )
    expect(screen.getByDisplayValue('JD Cloud · Standard video')).toBeTruthy()
  })

  test('a locked form cannot select an access method', () => {
    render(<Harness save={vi.fn()} disabled />)
    expect(
      (
        screen.getByRole('combobox', {
          name: 'Access method',
        }) as HTMLButtonElement
      ).disabled
    ).toBe(true)
  })
})
