import {
  QueryClient,
  QueryClientProvider,
  useQuery,
} from '@tanstack/react-query'
import { cleanup, render, screen } from '@testing-library/react'
import type { AxiosAdapter } from 'axios'
import { StrictMode } from 'react'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import {
  getBalance,
  getBalanceConnections,
  type BalanceConnection,
} from './api'

const originalAdapter = api.defaults.adapter
const clients: QueryClient[] = []

afterEach(() => {
  cleanup()
  clients.forEach((client) => client.clear())
  clients.length = 0
  api.defaults.adapter = originalAdapter
})

const connection: BalanceConnection = {
  id: 'test-reference',
  channel_id: 1,
  key_index: 0,
  key_label: '••••0001',
  origin: 'https://balance.example',
  url_key: 'https://balance.example',
  group_name: 'Balance upstream',
  channels: [],
  queryable: true,
}

function BalanceProbe(props: { kind: 'inventory' | 'balance' }) {
  const query = useQuery({
    queryKey: ['balance-cancellation', props.kind],
    queryFn: async ({ signal }) => {
      if (props.kind === 'inventory') {
        const rows = await getBalanceConnections(signal)
        return rows[0].origin
      }
      const balance = await getBalance(connection, signal)
      return balance.amounts?.[0].amount
    },
    retry: false,
  })
  if (query.isError) return <div>Load failed</div>
  return <div>{query.data ?? 'Loading'}</div>
}

it.each([
  ['inventory', connection.origin],
  ['balance', '12.34'],
] as const)(
  'loads %s after StrictMode cancels the first request',
  async (kind, expected) => {
    const adapter = vi.fn<AxiosAdapter>(async (config) => ({
      config,
      status: 200,
      statusText: 'OK',
      headers: {},
      data: {
        success: true,
        data:
          kind === 'inventory'
            ? [connection]
            : {
                status: 'ok',
                amounts: [{ amount: '12.34', unit: 'USD' }],
                checked_at: 1,
              },
      },
    }))
    api.defaults.adapter = adapter
    const client = new QueryClient()
    clients.push(client)
    render(
      <StrictMode>
        <QueryClientProvider client={client}>
          <BalanceProbe kind={kind} />
        </QueryClientProvider>
      </StrictMode>
    )
    expect(await screen.findByText(expected)).toBeInTheDocument()
    expect(screen.queryByText('Load failed')).not.toBeInTheDocument()
  }
)
