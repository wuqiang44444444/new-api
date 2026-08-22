import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { useForm } from 'react-hook-form'
import { I18nextProvider } from 'react-i18next'
import { beforeAll, describe, expect, test } from 'vitest'

import { Form } from '@/components/ui/form'

import {
  CHANNEL_FORM_DEFAULT_VALUES,
  type ChannelFormValues,
} from '../../../lib/channel-form'
import { ImageRelayProtocolFields } from '../image-relay-protocol-fields'

beforeAll(async () => {
  await i18next.init({
    lng: 'en',
    fallbackLng: 'en',
    resources: {
      en: {
        translation: {
          'Image Upstream Protocol': 'Image Upstream Protocol',
          'FunCloud Async Image V2': 'FunCloud Async Image V2',
          'Moxing Images V1': 'Moxing Images V1',
          'Select image protocol': 'Select image protocol',
          'Choose the code-backed image protocol. Models and model mapping remain administrator-managed.':
            'Choose the code-backed image protocol. Models and model mapping remain administrator-managed.',
        },
      },
    },
  })
})

function ImageRelayProtocolFieldsHarness({
  baseUrl = 'https://mm-internal-cn.leonecloud.com',
}: {
  baseUrl?: string
}) {
  const form = useForm<ChannelFormValues>({
    defaultValues: {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      type: 62,
      base_url: baseUrl,
      model_mapping: '{"customer":"provider"}',
      image_upstream_protocol: 'funcloud_aigc_v2',
    },
  })

  return (
    <I18nextProvider i18n={i18next}>
      <Form {...form}>
        <input aria-label='Base URL' {...form.register('base_url')} />
        <input aria-label='Model Mapping' {...form.register('model_mapping')} />
        <ImageRelayProtocolFields
          control={form.control}
          sensitiveLocked={false}
        />
      </Form>
    </I18nextProvider>
  )
}

describe('image relay protocol fields', () => {
  test('switches protocol and its known default URL without changing model mapping', async () => {
    const user = userEvent.setup()
    render(<ImageRelayProtocolFieldsHarness />)

    const trigger = screen.getByRole('combobox', {
      name: 'Image Upstream Protocol',
    })
    expect(trigger.textContent).toContain('FunCloud Async Image V2')

    await user.click(trigger)
    await user.click(
      await screen.findByRole('option', { name: 'Moxing Images V1' })
    )

    expect(trigger.textContent).toContain('Moxing Images V1')
    const baseUrlInput = screen.getByRole('textbox', {
      name: 'Base URL',
    }) as HTMLInputElement
    expect(baseUrlInput.value).toBe('https://www.moxing.pro')
    const modelMappingInput = screen.getByRole('textbox', {
      name: 'Model Mapping',
    }) as HTMLInputElement
    expect(modelMappingInput.value).toBe('{"customer":"provider"}')
  })

  test('preserves an administrator-supplied custom base URL', async () => {
    const user = userEvent.setup()
    render(
      <ImageRelayProtocolFieldsHarness baseUrl='https://image-proxy.example' />
    )

    await user.click(
      screen.getByRole('combobox', { name: 'Image Upstream Protocol' })
    )
    await user.click(
      await screen.findByRole('option', { name: 'Moxing Images V1' })
    )

    const baseUrlInput = screen.getByRole('textbox', {
      name: 'Base URL',
    }) as HTMLInputElement
    expect(baseUrlInput.value).toBe('https://image-proxy.example')
  })
})
