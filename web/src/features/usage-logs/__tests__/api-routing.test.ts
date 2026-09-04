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

import { getAllTaskLogs } from '../api'

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
