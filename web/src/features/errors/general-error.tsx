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
import { useNavigate, useRouter } from '@tanstack/react-router'
import { ArrowLeft, Home, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { useSystemConfig } from '@/hooks/use-system-config'
import { cn } from '@/lib/utils'

type GeneralErrorProps = React.HTMLAttributes<HTMLDivElement> & {
  minimal?: boolean
  error?: unknown
}

function getHttpStatus(error: unknown): number | undefined {
  if (typeof error !== 'object' || error === null) return undefined
  const response = (error as Record<string, unknown>).response
  if (typeof response !== 'object' || response === null) return undefined
  const status = (response as Record<string, unknown>).status
  return typeof status === 'number' ? status : undefined
}

export function GeneralError({
  className,
  minimal = false,
  error,
}: GeneralErrorProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { history } = useRouter()
  const { logo, systemName } = useSystemConfig()
  const status = getHttpStatus(error)
  const isRateLimited = status === 429
  const title = isRateLimited
    ? t('Too many requests')
    : t('Oops! Something went wrong')
  const description = isRateLimited
    ? t('Please wait a moment before trying again.')
    : t('Please try again later.')

  if (minimal) {
    return (
      <div
        className={cn(
          'flex min-h-48 w-full flex-col items-center justify-center gap-2 text-center',
          className
        )}
      >
        <span className='font-semibold'>{title}</span>
        <p className='text-muted-foreground text-sm'>{description}</p>
      </div>
    )
  }

  return (
    <div
      className={cn(
        'bg-background relative min-h-svh w-full overflow-hidden',
        className
      )}
    >
      <div
        aria-hidden='true'
        className='bg-primary/10 absolute -top-40 left-1/2 size-[34rem] -translate-x-1/2 rounded-full blur-3xl'
      />
      <div
        aria-hidden='true'
        className='absolute inset-0 [background-image:linear-gradient(to_right,currentColor_1px,transparent_1px),linear-gradient(to_bottom,currentColor_1px,transparent_1px)] [background-size:32px_32px] opacity-[0.035]'
      />

      <header className='relative z-10 mx-auto flex h-16 max-w-6xl items-center px-5 sm:px-8'>
        <button
          type='button'
          onClick={() => navigate({ to: '/' })}
          className='focus-visible:ring-ring flex items-center gap-2.5 rounded-lg outline-none focus-visible:ring-2'
        >
          <img
            src={logo}
            alt={t('Logo')}
            className='size-8 rounded-lg object-contain'
          />
          <span className='font-semibold tracking-tight'>{systemName}</span>
        </button>
      </header>

      <main className='relative z-10 mx-auto flex min-h-[calc(100svh-4rem)] max-w-6xl items-center justify-center px-5 pb-16 sm:px-8'>
        <section className='bg-card/90 ring-border/70 w-full max-w-2xl rounded-3xl border p-7 text-center shadow-[0_30px_80px_-40px_rgba(0,0,0,0.35)] ring-1 backdrop-blur sm:p-12'>
          <div className='bg-primary/10 text-primary mx-auto flex size-20 items-center justify-center rounded-2xl font-mono text-3xl font-semibold sm:size-24 sm:text-4xl'>
            {status ?? 500}
          </div>
          <p className='text-primary mt-7 text-sm font-semibold'>
            {systemName}
          </p>
          <h1 className='mt-2 text-3xl font-semibold tracking-tight sm:text-4xl'>
            {title}
          </h1>
          <p className='text-muted-foreground mx-auto mt-4 max-w-md leading-7'>
            {t('We apologize for the inconvenience.')} {description}
          </p>

          <div className='mt-8 flex flex-col-reverse justify-center gap-3 sm:flex-row'>
            <Button variant='outline' onClick={() => history.go(-1)}>
              <ArrowLeft aria-hidden='true' />
              {t('Go Back')}
            </Button>
            <Button variant='outline' onClick={() => window.location.reload()}>
              <RefreshCw aria-hidden='true' />
              {t('Refresh')}
            </Button>
            <Button onClick={() => navigate({ to: '/' })}>
              <Home aria-hidden='true' />
              {t('Back to Home')}
            </Button>
          </div>
        </section>
      </main>
    </div>
  )
}
