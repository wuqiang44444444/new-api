import { api } from '@/lib/api'

export type BalanceConnection = {
  id: string
  channel_id: number
  key_index: number
  key_label: string
  origin: string
  url_key: string
  group_name: string
  channels: { id: number; key_index: number; name: string; enabled: boolean }[]
  queryable: boolean
  reason?: string
}

export type BalanceResult = {
  status: 'ok' | 'error' | 'unsupported' | 'unavailable' | 'unlimited'
  reason?: string
  amounts?: { amount: string; unit: string; category?: string }[]
  scope?: 'upstream_balance' | 'unconfirmed' | 'credits'
  checked_at: number
}

export async function getBalanceConnections(signal?: AbortSignal) {
  const { data } = await api.get<{
    success: boolean
    data: BalanceConnection[]
  }>('/api/upstream-balances/', {
    signal,
    skipErrorHandler: true,
    // React Query owns deduplication and cancellation. The shared HTTP cache
    // otherwise reuses an aborted promise during StrictMode remounts.
    disableDuplicate: true,
  })
  if (!data.success || !Array.isArray(data.data)) {
    throw new Error('Unable to load upstream connections')
  }
  return data.data
}

// Bound requests in this browser as well as on the server. Queued requests check
// cancellation before using a credential, including after leaving the page.
const activeQueries = new Set<Promise<void>>()

export async function getBalance(
  connection: BalanceConnection,
  signal: AbortSignal
) {
  while (activeQueries.size >= 4) {
    await Promise.race(activeQueries)
    signal.throwIfAborted()
  }
  signal.throwIfAborted()
  const request = api.get<{ success: boolean; data: BalanceResult }>(
    `/api/upstream-balances/${connection.channel_id}/${connection.key_index}`,
    {
      params: { reference: connection.id },
      signal,
      skipErrorHandler: true,
      disableDuplicate: true,
    }
  )
  const slot = request.then(
    () => undefined,
    () => undefined
  )
  activeQueries.add(slot)
  try {
    const { data } = await request
    if (!data.success || !data.data) throw new Error('Balance query failed')
    return data.data
  } finally {
    activeQueries.delete(slot)
  }
}
