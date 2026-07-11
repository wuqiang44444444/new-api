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
import { ArrowRight } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { TokenAiMark } from '@/assets/token-ai-mark'
import { Skeleton } from '@/components/ui/skeleton'
import { useSystemConfig } from '@/hooks/use-system-config'
import { DEFAULT_LOGO } from '@/lib/constants'

import { AuthGatewayPreview } from './components/auth-gateway-preview'

type AuthLayoutProps = {
  children: React.ReactNode
}

export function AuthLayout({ children }: AuthLayoutProps) {
  const { t } = useTranslation()
  const { systemName, logo, loading, logoLoaded } = useSystemConfig()
  const hasCustomLogo = Boolean(logo && logo !== DEFAULT_LOGO)
  let brandMark: ReactNode

  if (loading || (hasCustomLogo && !logoLoaded)) {
    brandMark = <Skeleton className='absolute inset-0 rounded-lg' />
  } else if (hasCustomLogo) {
    brandMark = (
      <img
        src={logo}
        alt={t('Logo')}
        className='size-full rounded-lg object-cover'
      />
    )
  } else {
    brandMark = <TokenAiMark className='text-primary size-full' />
  }

  return (
    <div className='bg-background text-foreground relative min-h-svh'>
      <header className='absolute inset-x-0 top-0 z-10 flex items-center justify-between px-4 py-4 sm:px-8 sm:py-6 lg:px-12 xl:px-16'>
        <Link
          to='/'
          aria-label={t('Go to home')}
          className='focus-visible:ring-ring inline-flex min-h-11 items-center gap-2 rounded-lg transition-opacity outline-none hover:opacity-80 focus-visible:ring-2'
        >
          <span className='relative flex size-8 items-center justify-center overflow-hidden rounded-lg'>
            {brandMark}
          </span>
          {loading ? (
            <Skeleton className='h-6 w-24' />
          ) : (
            <span className='text-xl font-semibold'>{systemName}</span>
          )}
        </Link>

        <Link
          to='/'
          className='text-primary focus-visible:ring-ring inline-flex min-h-11 items-center gap-1 rounded-lg px-2 text-sm font-medium outline-none hover:opacity-75 focus-visible:ring-2'
        >
          {t('Go to home')}
          <ArrowRight aria-hidden='true' className='size-4' />
        </Link>
      </header>

      <main className='grid min-h-svh lg:grid-cols-[minmax(0,46%)_minmax(0,54%)]'>
        <section className='flex min-w-0 items-center px-4 pt-24 pb-10 sm:px-8 sm:pt-28 lg:px-12 xl:px-20'>
          <div className='mx-auto w-full max-w-[29rem]'>{children}</div>
        </section>

        <aside className='bg-muted/40 border-border hidden min-w-0 items-center border-l px-10 pt-28 pb-16 lg:flex xl:px-16'>
          <AuthGatewayPreview systemName={systemName} />
        </aside>
      </main>
    </div>
  )
}
