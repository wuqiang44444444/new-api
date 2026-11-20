/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { afterEach, describe, expect, test } from 'vitest'

import { api } from '@/lib/api'

import { getAllTaskLogs, getUsageLogDetail } from '../api'

const originalGet = api.get

afterEach(() => {
  api.get = originalGet
})

describe('task log API routing', () => {
  test('uses the canonical trailing-slash route for the admin task list', async () => {
    let requestedUrl: string | undefined
    api.get = ((url: string) => {
      requestedUrl = url
      return Promise.resolve({ data: {} })
    }) as typeof api.get

    await getAllTaskLogs({ p: 2, page_size: 50, task_id: 'task-1' })

    expect(requestedUrl).toBe('/api/task/?p=2&page_size=50&task_id=task-1')
  })
})

test.each([
  [true, '/api/log/console/page'],
  [false, '/api/log/self/console/page'],
] as const)(
  'uses the canonical detail route for admin=%s',
  async (isAdmin, path) => {
    const signal = new AbortController().signal
    let requestedUrl: string | undefined
    api.get = ((url, config) => {
      requestedUrl = api.getUri({ params: config?.params, url })
      return Promise.resolve({ data: { success: true, data: { id: 1 } } })
    }) as typeof api.get

    await getUsageLogDetail('test-log-reference', isAdmin, signal, 1600000000)

    expect(requestedUrl).toBe(`${path}?log_ref=test-log-reference`)
  }
)

test('bounds a 404 fallback and refuses an ordinary list response', async () => {
  const signal = new AbortController().signal
  const requests: string[] = []
  api.get = ((url, config) => {
    requests.push(api.getUri({ params: config?.params, url }))
    return Promise.resolve(
      requests.length === 1
        ? { status: 404, data: {} }
        : { status: 200, data: { success: true, data: { items: [{ id: 1 }] } } }
    )
  }) as typeof api.get

  await expect(
    getUsageLogDetail('test-log-reference', true, signal, 1600000000)
  ).rejects.toThrow()
  expect(requests).toEqual([
    '/api/log/console/page?log_ref=test-log-reference',
    '/api/log/?log_ref=test-log-reference&start_timestamp=1600000000&end_timestamp=1600000000&page_size=1',
  ])
})

test('can read the same authorized detail from a snapshot-capable older instance', async () => {
  const createdAt = 1600000000
  const detail = {
    id: 1,
    user_id: 7,
    created_at: createdAt,
    type: 2,
    content: 'detail',
  }
  api.get = ((url) =>
    Promise.resolve(
      url.endsWith('/console/page')
        ? { status: 404, data: {} }
        : { status: 200, data: { success: true, data: detail } }
    )) as typeof api.get

  const signal = new AbortController().signal
  await expect(
    getUsageLogDetail('test-log-reference', false, signal, createdAt)
  ).resolves.toEqual(detail)
})
