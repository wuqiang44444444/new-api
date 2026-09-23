import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ComponentProps } from 'react'
import { describe, expect, it, vi } from 'vitest'

import { CustomerContractAddRule } from '../user-contract-add-rule'

function propsForSelection(): ComponentProps<typeof CustomerContractAddRule> {
  return {
    channelGroups: [
      {
        group: 'default',
        native_group_ratio: '1',
        special_group_ratio: false,
        models: [
          {
            model: 'model-a',
            channels: [
              { id: 1, name: 'Primary channel with a long descriptive name' },
              { id: 2, name: 'Backup channel' },
              { id: 3, name: 'Regional channel' },
            ],
          },
        ],
      },
    ],
    group: 'default',
    models: ['model-a'],
    channelIdsByModel: { 'model-a': ['1', '2', '3'] },
    discount: '0.8',
    onGroupChange: vi.fn(),
    onModelsChange: vi.fn(),
    onModelChannelsChange: vi.fn(),
    onDiscountChange: vi.fn(),
    onAdd: vi.fn(),
  }
}

describe('contract rule selection layout', () => {
  it('keeps multiple channel selections compact and exposes every choice in the dropdown', async () => {
    const props = propsForSelection()
    render(<CustomerContractAddRule {...props} />)
    expect(screen.getByText('3 selected')).toBeVisible()
    expect(
      screen.queryByText('Primary channel with a long descriptive name')
    ).not.toBeInTheDocument()

    const user = userEvent.setup()
    await user.click(
      screen.getByRole('combobox', { name: 'Channels for model model-a' })
    )
    expect(
      await screen.findByRole('option', {
        name: 'Primary channel with a long descriptive name',
      })
    ).toHaveAttribute('aria-selected', 'true')
    await user.click(screen.getByRole('option', { name: 'Backup channel' }))
    expect(props.onModelChannelsChange).toHaveBeenCalledWith('model-a', [
      '1',
      '3',
    ])
  })

  it('places the add action after channel selection with an announced rule count', () => {
    const props = propsForSelection()
    render(<CustomerContractAddRule {...props} />)
    const channelInput = screen.getByRole('combobox', {
      name: 'Channels for model model-a',
    })
    const add = screen.getByRole('button', { name: 'Add' })
    expect(
      channelInput.compareDocumentPosition(add) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
    expect(screen.getByRole('status')).toHaveTextContent(
      'Will add 1 models and 3 rules'
    )
    fireEvent.click(add)
    expect(props.onAdd).toHaveBeenCalledOnce()
  })

  it('associates the visible model and group labels with their controls', () => {
    render(<CustomerContractAddRule {...propsForSelection()} />)
    expect(screen.getByLabelText('Route group')).toHaveAttribute(
      'role',
      'combobox'
    )
    expect(screen.getByLabelText('Model')).toHaveAttribute('role', 'combobox')
  })

  it('keeps a single selected channel name visible and disables adding an empty selection', () => {
    const props = propsForSelection()
    const { rerender } = render(
      <CustomerContractAddRule
        {...props}
        channelIdsByModel={{ 'model-a': ['1'] }}
      />
    )
    expect(
      screen.getByText('Primary channel with a long descriptive name')
    ).toBeVisible()
    rerender(
      <CustomerContractAddRule {...props} models={[]} channelIdsByModel={{}} />
    )
    expect(screen.getByRole('button', { name: 'Add' })).toBeDisabled()
    expect(
      screen.queryByRole('combobox', { name: 'Channels for model model-a' })
    ).not.toBeInTheDocument()
  })
})
