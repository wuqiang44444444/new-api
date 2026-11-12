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
import type { ComponentProps } from 'react'

import { Table } from '@/components/ui/table'
import { cn } from '@/lib/utils'

export function CustomerBillingTable(props: ComponentProps<typeof Table>) {
  return (
    <Table
      {...props}
      className={cn(
        '[&_th]:bg-(--table-header) [&_th]:text-muted-foreground [&_th]:px-3 [&_th]:text-xs [&_th]:font-semibold [&_td]:px-3 [&_td]:font-normal [&_tbody>tr]:h-14 [&_tbody>tr:nth-child(even):not(:hover)]:bg-muted/20',
        'max-sm:[&_[data-column=actions]]:sticky max-sm:[&_[data-column=actions]]:right-0 max-sm:[&_[data-column=actions]]:border-l max-sm:[&_td[data-column=actions]]:bg-card',
        props.className
      )}
    />
  )
}
