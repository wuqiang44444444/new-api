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
import type { SVGProps } from 'react'

import { cn } from '@/lib/utils'

export function Logo({ className, ...props }: SVGProps<SVGSVGElement>) {
  return (
    <svg
      id='tokenai-logo'
      viewBox='0 0 24 24'
      xmlns='http://www.w3.org/2000/svg'
      height='24'
      width='24'
      fill='none'
      stroke='currentColor'
      strokeWidth='2'
      strokeLinejoin='round'
      className={cn('size-6', className)}
      {...props}
    >
      <title>TokenAI</title>
      {/* App tile */}
      <rect x='3' y='3' width='18' height='18' rx='5.5' />
      {/* 4-point AI spark */}
      <path
        d='M12 6.4 L13.25 10.75 L17.6 12 L13.25 13.25 L12 17.6 L10.75 13.25 L6.4 12 L10.75 10.75 Z'
        fill='currentColor'
        stroke='none'
      />
    </svg>
  )
}
