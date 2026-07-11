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
import { Link } from '@tanstack/react-router'
import {
  ArrowRight,
  ReceiptText,
  RefreshCw,
  ShieldCheck,
  Shuffle,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/design-system/button'

interface CTAProps {
  className?: string
  isAuthenticated?: boolean
}

const ENTERPRISE_CAPABILITIES = [
  {
    title: 'Multi-channel scheduling',
    description: 'Reduce single-channel volatility',
    icon: Shuffle,
  },
  {
    title: 'Failure retries and rate limits',
    description: 'Protect request availability boundaries',
    icon: RefreshCw,
  },
  {
    title: 'Permissions and quotas',
    description: 'Manage team access centrally',
    icon: ShieldCheck,
  },
  {
    title: 'Transparent billing and logs',
    description: 'Verify token usage and charges',
    icon: ReceiptText,
  },
] as const

export function CTA(props: CTAProps) {
  const { t } = useTranslation()

  return (
    <section
      id='enterprise'
      className='border-border scroll-mt-20 border-t px-4 py-20 sm:px-6 sm:py-24'
      aria-labelledby='enterprise-title'
    >
      <div className='mx-auto grid max-w-6xl items-center gap-12 lg:grid-cols-[0.95fr_1.05fr] lg:gap-20'>
        <div className='max-w-xl'>
          <p className='text-primary text-sm font-semibold'>
            {t('Enterprise AI Gateway')}
          </p>
          <h2
            id='enterprise-title'
            className='mt-3 text-[clamp(1.75rem,3vw,2.375rem)] leading-tight font-semibold tracking-[-0.035em]'
          >
            {t('Move AI applications from testing to continuous operations')}
          </h2>
          <p className='text-muted-foreground mt-4 text-base leading-7'>
            {t(
              'Team billing, technical support, and gateway deployment services help enterprises manage model access, permissions, quotas, and request quality through one entry point.'
            )}
          </p>
          <div className='mt-7 flex flex-wrap items-center gap-3'>
            <Button
              size='xl'
              render={
                <Link to={props.isAuthenticated ? '/dashboard' : '/sign-up'} />
              }
            >
              {props.isAuthenticated ? t('Go to Dashboard') : t('Get Started')}
            </Button>
            <Button
              variant='ghost'
              size='xl'
              render={
                <a
                  href='https://docs.newapi.pro/installation/'
                  target='_blank'
                  rel='noopener noreferrer'
                />
              }
            >
              {t('Learn about gateway deployment')}
              <ArrowRight data-icon='inline-end' />
            </Button>
          </div>
        </div>

        <ul className='border-border border-t'>
          {ENTERPRISE_CAPABILITIES.map((capability) => {
            const Icon = capability.icon
            return (
              <li
                key={capability.title}
                className='border-border grid min-h-16 grid-cols-[2rem_1fr] items-center gap-3 border-b py-3 sm:grid-cols-[2rem_1fr_auto]'
              >
                <Icon aria-hidden='true' className='text-primary size-5' />
                <strong className='text-sm font-semibold'>
                  {t(capability.title)}
                </strong>
                <span className='text-muted-foreground col-start-2 text-xs sm:col-start-auto'>
                  {t(capability.description)}
                </span>
              </li>
            )
          })}
        </ul>
      </div>
    </section>
  )
}
