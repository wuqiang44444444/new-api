import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ComponentProps } from 'react'
import { describe, expect, it, vi } from 'vitest'

import type { CustomerContractCatalog } from '../../types'
import { CustomerContractAddRule } from '../user-contract-add-rule'

const availableSource = (channelId: number, name: string) => ({
  channel_id: channelId,
  channel_name: name,
  available: true,
  from_channel_config: true,
  from_ability: true,
})

function catalogFixture(): CustomerContractCatalog {
  return {
    groups: [
      {
        group: 'default',
        ratio_configured: true,
        native_group_ratio: '1',
        special_group_ratio: false,
        models: [
          {
            model: 'model-a',
            sources: [
              availableSource(
                1,
                'Primary channel with a long descriptive name'
              ),
              availableSource(2, 'Backup channel'),
              availableSource(3, 'Regional channel'),
            ],
          },
        ],
      },
      {
        group: 'other-group',
        ratio_configured: true,
        native_group_ratio: '1',
        special_group_ratio: false,
        models: [
          {
            model: 'model-elsewhere',
            sources: [availableSource(9, 'Distant channel')],
          },
        ],
      },
    ],
    no_group_channels: [
      {
        channel_id: 7,
        channel_name: 'Orphan channel',
        channel_status: 1,
        models: ['orphan-model'],
      },
    ],
    customer_context: true,
  }
}

function propsForSelection(): ComponentProps<typeof CustomerContractAddRule> {
  return {
    catalog: catalogFixture(),
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

  it('keeps the original group and model controls without a separate discovery step', () => {
    render(<CustomerContractAddRule {...propsForSelection()} />)
    expect(screen.getByLabelText('Route group')).toHaveAttribute(
      'role',
      'combobox'
    )
    expect(screen.getByLabelText('Model')).toHaveAttribute('role', 'combobox')
    expect(
      screen.queryByLabelText('Discover model in all route groups')
    ).not.toBeInTheDocument()
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

describe('group, model, then channel selection', () => {
  it('offers configured H3 and M3 without requiring Ability or pricing, and selects all group models despite a search', async () => {
    const props = propsForSelection()
    props.catalog.groups[0]?.models.push(
      ...['MiniMax-H3', 'MiniMax-M3'].map((model) => ({
        model,
        sources: [{ ...availableSource(8, 'MiniMax'), from_ability: false }],
      }))
    )
    const user = userEvent.setup()
    const { rerender } = render(
      <CustomerContractAddRule {...props} models={[]} channelIdsByModel={{}} />
    )
    const models = screen.getByLabelText('Model')
    await user.click(models)
    await user.type(models, 'MiniMax-H3')
    expect(
      await screen.findByRole('option', { name: 'MiniMax-H3' })
    ).toBeVisible()
    expect(
      screen.queryByRole('option', { name: 'model-elsewhere' })
    ).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Select all' }))
    expect(props.onModelsChange).toHaveBeenCalledWith([
      'model-a',
      'MiniMax-H3',
      'MiniMax-M3',
    ])
    await user.keyboard('{Escape}')
    rerender(
      <CustomerContractAddRule
        {...props}
        models={['MiniMax-H3']}
        channelIdsByModel={{}}
      />
    )
    await user.click(
      screen.getByRole('combobox', { name: 'Channels for model MiniMax-H3' })
    )
    await user.click(await screen.findByRole('option', { name: 'MiniMax' }))
    expect(props.onModelChannelsChange).toHaveBeenCalledWith('MiniMax-H3', [
      '8',
    ])
  })

  it('selects every available channel for a model even when the channel search is filtered', async () => {
    const props = propsForSelection()
    const user = userEvent.setup()
    render(<CustomerContractAddRule {...props} channelIdsByModel={{}} />)
    const channels = screen.getByRole('combobox', {
      name: 'Channels for model model-a',
    })
    await user.click(channels)
    await user.type(channels, 'Backup')
    await user.click(screen.getByRole('button', { name: 'Select all' }))
    expect(props.onModelChannelsChange).toHaveBeenCalledWith('model-a', [
      '1',
      '2',
      '3',
    ])
  })

  it('keeps configured models selectable while unavailable channels cannot be selected', async () => {
    const props = propsForSelection()
    props.catalog.groups[0]?.models.push({
      model: 'model-down',
      sources: [
        {
          ...availableSource(5, 'Stopped channel'),
          available: false,
          unavailable_reason: 'channel_disabled',
        },
      ],
    })
    const user = userEvent.setup()
    const { rerender } = render(
      <CustomerContractAddRule {...props} models={[]} channelIdsByModel={{}} />
    )
    await user.click(screen.getByLabelText('Model'))
    await user.click(await screen.findByRole('option', { name: 'model-down' }))
    expect(props.onModelsChange).toHaveBeenCalledWith(['model-down'])
    await user.keyboard('{Escape}')
    rerender(
      <CustomerContractAddRule
        {...props}
        models={['model-down']}
        channelIdsByModel={{}}
      />
    )
    expect(
      screen.getByText('No qualifying channel is available for this model')
    ).toBeVisible()
    await user.click(
      screen.getByRole('combobox', { name: 'Channels for model model-down' })
    )
    expect(
      screen.queryByRole('option', { name: 'Stopped channel' })
    ).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Select all' })).toBeDisabled()
    expect(props.onModelChannelsChange).not.toHaveBeenCalled()
  })

  it('clears pending models when the route group changes', async () => {
    const props = propsForSelection()
    const user = userEvent.setup()
    render(<CustomerContractAddRule {...props} />)
    await user.click(screen.getByLabelText('Route group'))
    await user.click(await screen.findByRole('option', { name: 'other-group' }))
    expect(props.onGroupChange).toHaveBeenCalledWith('other-group')
    expect(props.onModelsChange).toHaveBeenCalledWith([])
  })
})
