import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'

import {
  CHANNEL_TYPE_MINIMAX_LINK,
  CHANNEL_TYPE_SEEDANCE_LINK,
} from '../../constants'
import { channelSchema, type Channel } from '../../types'
import { useChannelsColumns } from '../channels-columns'
import { ChannelsProvider } from '../channels-provider'

function RoutingCells(props: { channel: Channel }) {
  const columns = useChannelsColumns()
  const table = useReactTable({
    data: [props.channel],
    columns,
    getCoreRowModel: getCoreRowModel(),
  })
  return (
    <>
      {table
        .getRowModel()
        .rows[0].getAllCells()
        .filter((cell) => ['priority', 'weight'].includes(cell.column.id))
        .map((cell) => (
          <div key={cell.id}>
            {flexRender(cell.column.columnDef.cell, cell.getContext())}
          </div>
        ))}
    </>
  )
}

test.each([
  ['native MiniMax', 35, false, true],
  ['standard MiniMax', CHANNEL_TYPE_MINIMAX_LINK, false, false],
  ['Seedance', CHANNEL_TYPE_SEEDANCE_LINK, false, false],
  ['mixed tag', CHANNEL_TYPE_MINIMAX_LINK, true, false],
] as const)(
  '%s exposes routing edits only when they apply',
  (_name, type, aggregate, editable) => {
    const channel = channelSchema.parse({
      id: 1,
      type,
      key: '',
      status: 1,
      name: 'Test',
      created_time: 0,
      test_time: 0,
      response_time: 0,
      balance_updated_time: 0,
    })
    const row = aggregate
      ? {
          ...channel,
          type: 35,
          tag: 'mixed',
          children: [channel, { ...channel, id: 2, type: 35 }],
        }
      : channel
    const client = new QueryClient()
    render(
      <QueryClientProvider client={client}>
        <ChannelsProvider>
          <RoutingCells channel={row} />
        </ChannelsProvider>
      </QueryClientProvider>
    )
    if (editable) {
      expect(screen.queryByText('Not applicable')).not.toBeInTheDocument()
      expect(screen.getAllByRole('button').length).toBeGreaterThan(0)
    } else {
      expect(screen.getAllByText('Not applicable')).toHaveLength(2)
      expect(screen.queryByRole('button')).not.toBeInTheDocument()
    }
  }
)
