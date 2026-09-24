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
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { UserInfoDialog } from '../dialogs/user-info-dialog'

beforeEach(() => {
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
})

it.each([
  { usedQuota: null, expected: 'Usage needs review' },
  { usedQuota: 500000, expected: '$1' },
])(
  'shows $expected for returned usage $usedQuota without hiding user details',
  async ({ usedQuota, expected }) => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: {
          id: 91,
          username: 'usage-user',
          quota: 2000000,
          used_quota: usedQuota,
          request_count: 12,
        },
      },
    })
    render(<UserInfoDialog userId={91} open onOpenChange={vi.fn()} />)
    expect(await screen.findByText(expected)).toBeInTheDocument()
    expect(screen.getByText('usage-user')).toBeInTheDocument()
    expect(screen.getByText('$4')).toBeInTheDocument()
    expect(
      screen.queryByText('No user information available')
    ).not.toBeInTheDocument()
  }
)
