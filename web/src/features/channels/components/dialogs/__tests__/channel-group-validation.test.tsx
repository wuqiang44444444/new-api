import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterAll, beforeAll, describe, expect, test, vi } from 'vitest'

import { EditTagDialog } from '../edit-tag-dialog'
import { TagBatchEditDialog } from '../tag-batch-edit-dialog'

const api = vi.hoisted(() => ({
  editTagChannels: vi.fn(async () => ({ success: true })),
}))

vi.mock('../../../api', () => ({
  editTagChannels: api.editTagChannels,
  getGroups: async () => ({ success: true, data: ['a'.repeat(127), 'b'] }),
  getTagModels: async () => ({ success: true, data: '' }),
  getAllModels: async () => ({ success: true, data: [] }),
}))

vi.mock('../../channels-provider', () => ({
  useChannels: () => ({ currentTag: 'fixture-tag' }),
}))

const originalGetAnimations = HTMLElement.prototype.getAnimations
beforeAll(() => {
  HTMLElement.prototype.getAnimations = () => []
})
afterAll(() => {
  HTMLElement.prototype.getAnimations = originalGetAnimations
})

describe('tag group list validation', () => {
  test('tag editor displays an inline error and disables saving an oversized list', async () => {
    const user = userEvent.setup()
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={client}>
        <EditTagDialog open onOpenChange={vi.fn()} />
      </QueryClientProvider>
    )
    await user.click(await screen.findByText('a'.repeat(127)))
    await user.click(screen.getByText('b'))
    expect(screen.getByRole('alert', { name: 'Groups' })).toHaveTextContent(
      '128'
    )
    expect(screen.getByRole('button', { name: 'Save Changes' })).toBeDisabled()
    await user.click(screen.getByText('b'))
    expect(
      screen.queryByRole('alert', { name: 'Groups' })
    ).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Save Changes' })).toBeEnabled()
  })

  test('batch tag editor blocks saving until a long list is shortened', async () => {
    const user = userEvent.setup()
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={client}>
        <TagBatchEditDialog open onOpenChange={vi.fn()} />
      </QueryClientProvider>
    )
    const input = await screen.findByRole('combobox')
    await user.click(input)
    await user.click(
      await screen.findByRole('option', { name: 'a'.repeat(127) })
    )
    await user.click(input)
    await user.click(await screen.findByRole('option', { name: 'b' }))
    await user.keyboard('{Escape}')
    await waitFor(() =>
      expect(screen.getByRole('alert', { name: 'Groups' })).toHaveTextContent(
        '128'
      )
    )
    expect(screen.getByRole('button', { name: 'Save Changes' })).toBeDisabled()
  })
})
