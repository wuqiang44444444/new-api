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
import { BadgePercent, CircleCheck, CircleOff, ShieldX } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

import type { CustomerContractAdminSummary } from '../types'

interface CustomerContractSummaryProps {
  summary: CustomerContractAdminSummary
  isLoading: boolean
}

export function CustomerContractSummary(props: CustomerContractSummaryProps) {
  const { t } = useTranslation()
  const cards = [
    {
      key: 'total',
      label: t('All contracts'),
      value: props.summary.total,
      icon: BadgePercent,
      iconClassName: 'text-primary',
    },
    {
      key: 'active',
      label: t('Active contracts'),
      value: props.summary.active,
      icon: CircleCheck,
      iconClassName: 'text-success',
    },
    {
      key: 'zero-access',
      label: t('No model access'),
      value: props.summary.zero_access,
      icon: ShieldX,
      iconClassName: 'text-destructive',
    },
    {
      key: 'inactive',
      label: t('Inactive contracts'),
      value: props.summary.inactive,
      icon: CircleOff,
      iconClassName: 'text-muted-foreground',
    },
  ]

  return (
    <div className='grid shrink-0 grid-cols-2 gap-2.5 lg:grid-cols-4'>
      {cards.map((card) => {
        const Icon = card.icon
        return (
          <Card key={card.key} size='sm'>
            <CardHeader>
              <CardDescription>{card.label}</CardDescription>
              <CardAction>
                <Icon
                  aria-hidden='true'
                  className={`size-4 ${card.iconClassName}`}
                />
              </CardAction>
            </CardHeader>
            <CardContent>
              {props.isLoading ? (
                <Skeleton className='h-7 w-14' />
              ) : (
                <div className='text-2xl font-semibold tabular-nums'>
                  {card.value}
                </div>
              )}
            </CardContent>
          </Card>
        )
      })}
    </div>
  )
}
