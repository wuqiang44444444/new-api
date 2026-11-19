import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { createRef, useState } from 'react'
import { describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import {
  ImageSizePricingEditor,
  type ImagePriceGridHandle,
} from '../image-size-pricing-editor'

vi.mock('i18next', () => ({ t: (key: string) => key }))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

const profile = {
  profile: 'size_only',
  prices: { '1K_default': 0.01, '2K_default': 0.02, '4K_default': 0.04 },
  qualities: ['default'],
  size_tiers: ['1K', '2K', '4K'],
  default_quality: 'default',
  unmatched_uses_quality: false,
}

function renderEditor(expression: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const handle = createRef<ImagePriceGridHandle>()
  const onDirtyChange = vi.fn()
  function Editor() {
    const [body, setBody] = useState(expression)
    return (
      <QueryClientProvider client={client}>
        <ImageSizePricingEditor
          ref={handle}
          expression={body}
          onExpressionChange={setBody}
          onDirtyChange={onDirtyChange}
        >
          <input aria-label='Hand-written expression' value={body} readOnly />
        </ImageSizePricingEditor>
      </QueryClientProvider>
    )
  }
  render(<Editor />)
  return { handle, onDirtyChange }
}

describe('image size price editor', () => {
  it('reopens canonical prices and generates the expression from edited amounts on save', async () => {
    const request = vi.spyOn(api, 'post')
    request.mockResolvedValueOnce({
      data: { success: true, data: { table: profile } },
    })
    request.mockResolvedValueOnce({
      data: { success: true, data: { expression: 'generated' } },
    })
    const { handle, onDirtyChange } = renderEditor(
      'tier("saved", fixed(1)) * image_count'
    )
    const price = await screen.findByRole('textbox', { name: '4K default' })
    expect(price).toHaveValue('0.04')
    expect(
      screen.queryByRole('textbox', { name: 'Hand-written expression' })
    ).toBeNull()
    fireEvent.change(price, { target: { value: '0.06' } })
    await waitFor(() => expect(onDirtyChange).toHaveBeenLastCalledWith(true))
    await act(async () =>
      expect(await handle.current?.commit()).toBe('generated')
    )
    expect(request).toHaveBeenLastCalledWith(
      '/api/option/billing-expression/image-prices',
      {
        profile: 'size_only',
        prices: { '1K_default': 0.01, '2K_default': 0.02, '4K_default': 0.06 },
      }
    )
    request.mockRestore()
  })

  it('imports recognized legacy prices that vary by quality alone', async () => {
    const prices = Object.fromEntries(
      ['1K', '2K', '4K'].flatMap((size) =>
        Object.entries({ low: 0.1, medium: 0.2, high: 0.3 }).map(
          ([quality, price]) => [`${size}_${quality}`, price]
        )
      )
    )
    const table = {
      ...profile,
      profile: 'medium_three',
      qualities: ['low', 'medium', 'high'],
      default_quality: 'medium',
      prices,
    }
    const post = vi
      .spyOn(api, 'post')
      .mockResolvedValueOnce({
        data: { success: true, data: { importable: true, warnings: [] } },
      })
      .mockResolvedValueOnce({ data: { success: true, data: { table } } })
      .mockResolvedValueOnce({
        data: { success: true, data: { expression: 'converted' } },
      })
    const { handle } = renderEditor('legacy * image_count')
    fireEvent.click(
      await screen.findByRole('button', {
        name: 'Import existing image prices',
      })
    )
    expect(await screen.findByRole('textbox', { name: '4K high' })).toHaveValue(
      '0.3'
    )
    expect(screen.queryByRole('alert')).toBeNull()
    await act(async () =>
      expect(await handle.current?.commit()).toBe('converted')
    )
    expect(post).toHaveBeenLastCalledWith(
      '/api/option/billing-expression/image-prices',
      { profile: 'medium_three', prices }
    )
    post.mockRestore()
  })

  it('carries edited prices into the raw expression before leaving the grid', async () => {
    const updated = {
      ...profile,
      prices: { ...profile.prices, '4K_default': 0.06 },
    }
    const post = vi
      .spyOn(api, 'post')
      .mockImplementation(async (_url, payload) => {
        const req = payload as { profile?: string; expression?: string }
        const data = req.profile
          ? { expression: 'updated * image_count' }
          : {
              table:
                req.expression === 'updated * image_count' ? updated : profile,
            }
        return { data: { success: true, data } }
      })
    renderEditor('original * image_count')
    fireEvent.change(
      await screen.findByRole('textbox', { name: '4K default' }),
      { target: { value: '0.06' } }
    )
    fireEvent.click(screen.getByRole('button', { name: 'Edit expression' }))
    expect(
      await screen.findByRole('textbox', { name: 'Hand-written expression' })
    ).toHaveValue('updated * image_count')
    await waitFor(() =>
      expect(post).toHaveBeenCalledWith(expect.any(String), {
        expression: 'updated * image_count',
      })
    )
    fireEvent.click(
      screen.getByRole('button', { name: 'Size tiers per image' })
    )
    expect(
      await screen.findByRole('textbox', { name: '4K default' })
    ).toHaveValue('0.06')
    post.mockRestore()
  })

  it('shows warnings for hand-written pricing without blocking the existing save path', async () => {
    const request = vi.spyOn(api, 'post').mockResolvedValueOnce({
      data: {
        success: true,
        data: {
          warnings: [{ code: 'unavailable_parameter', field: 'resolution' }],
        },
      },
    })
    const { handle } = renderEditor('tier("manual", fixed(1)) * image_count')
    await screen.findByText(
      'The native per-image billing input does not include {{field}}.'
    )
    expect(
      screen.getByRole('textbox', { name: 'Hand-written expression' })
    ).toBeVisible()
    expect(await handle.current?.commit()).toBeUndefined()
    request.mockRestore()
  })
})
