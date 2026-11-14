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
import { createFileRoute, getRouteApi } from '@tanstack/react-router'
import { z } from 'zod'

import { ErrorLogs } from '@/features/usage-logs/error'

const errorLogSearch = z.object({
  start_timestamp: z
    .number()
    .int()
    .positive()
    .max(253402300799)
    .optional()
    .catch(undefined),
  end_timestamp: z
    .number()
    .int()
    .positive()
    .max(253402300799)
    .optional()
    .catch(undefined),
  event_type: z
    .enum(['api_error', 'channel_test', 'stream_error', 'task_failure'])
    .optional()
    .catch(undefined),
  channel: z
    .number()
    .int()
    .positive()
    .max(Number.MAX_SAFE_INTEGER)
    .optional()
    .catch(undefined),
  status: z.number().int().min(100).max(599).optional().catch(undefined),
  model_name: z.string().min(1).max(256).optional().catch(undefined),
  reason: z.string().min(1).max(128).optional().catch(undefined),
})

export const Route = createFileRoute('/_authenticated/usage-logs/error')({
  validateSearch: errorLogSearch,
  component: ErrorLogsRoute,
})

function ErrorLogsRoute() {
  const search = getRouteApi('/_authenticated/usage-logs/error').useSearch()
  return (
    <ErrorLogs
      key={JSON.stringify(search)}
      initialFilters={{
        ...search,
        channel: search.channel?.toString(),
        status: search.status?.toString(),
      }}
    />
  )
}
