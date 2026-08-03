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
import { ArrowRight, BookOpen, CircleCheck } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { useStatus } from '@/hooks/use-status'
import { useSystemConfig } from '@/hooks/use-system-config'

import { resolveHomeBaseUrl } from '../../lib/server-address'
import { GatewayFlow } from '../gateway-flow'

interface HeroProps {
  className?: string
  isAuthenticated?: boolean
}

export function Hero(props: HeroProps) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const { systemName } = useSystemConfig()
  const serverAddress = resolveHomeBaseUrl(status)
  const docsUrl =
    (status?.docs_link as string | undefined) || 'https://docs.newapi.pro'

  const docsButton = docsUrl.startsWith('http') ? (
    <Button
      variant='outline'
      className='h-11 gap-1.5 px-5'
      render={
        <a href={docsUrl} target='_blank' rel='noopener noreferrer' />
      }
    >
      <BookOpen aria-hidden='true' className='size-4' />
      {t('Docs')}
    </Button>
  ) : (
    <Button
      variant='outline'
      className='h-11 gap-1.5 px-5'
      render={<Link to={docsUrl} />}
    >
      <BookOpen aria-hidden='true' className='size-4' />
      {t('Docs')}
    </Button>
  )

  return (
    <section
      className='relative overflow-hidden px-4 pt-28 pb-12 sm:px-6 sm:pt-32 sm:pb-16'
      aria-labelledby='home-hero-title'
    >
      <div className='relative mx-auto grid max-w-6xl items-center gap-14 lg:grid-cols-[minmax(0,1fr)_minmax(460px,0.9fr)] lg:gap-20'>
        <div className='max-w-2xl py-4'>
          <p className='text-primary mb-5 text-sm font-semibold'>
            {t('Professional Token Service & Enterprise AI Gateway')}
          </p>
          <h1
            id='home-hero-title'
            className='text-[clamp(2.5rem,5vw,3.5rem)] leading-[1.12] font-semibold tracking-[-0.045em]'
          >
            {t('Reliable AI model access.')}
            <span className='text-primary block'>
              {t('Enterprise-ready by design.')}
            </span>
          </h1>
          <p className='text-muted-foreground mt-6 max-w-xl text-base leading-8'>
            {t(
              '{{systemName}} unifies leading text, image, audio, and video models behind one API, with resilient routing, usage controls, transparent billing, and end-to-end observability.',
              { systemName }
            )}
          </p>
          <ul
            className='text-muted-foreground mt-6 flex flex-wrap gap-x-5 gap-y-2 text-sm font-medium'
            aria-label={t('Core capabilities')}
          >
            {[
              'Unified API access',
              'Resilient multi-channel routing',
              'Traceable token usage and billing',
            ].map((item) => (
              <li key={item} className='flex items-center gap-2'>
                <CircleCheck
                  aria-hidden='true'
                  className='text-primary size-4'
                />
                {t(item)}
              </li>
            ))}
          </ul>
          <div className='mt-8 flex flex-wrap items-center gap-3'>
            {props.isAuthenticated ? (
              <>
                <Button className='h-11 px-5' render={<Link to='/dashboard' />}>
                  {t('Go to Dashboard')}
                  <ArrowRight data-icon='inline-end' />
                </Button>
                {docsButton}
              </>
            ) : (
              <>
                <Button className='h-11 px-5' render={<Link to='/sign-up' />}>
                  {t('Get Started')}
                  <ArrowRight data-icon='inline-end' />
                </Button>
                <Button
                  variant='outline'
                  className='h-11 px-5'
                  render={<Link to='/pricing' />}
                >
                  {t('View Pricing')}
                </Button>
                {docsButton}
              </>
            )}
          </div>
        </div>

        <GatewayFlow systemName={systemName} serverAddress={serverAddress} />
      </div>
    </section>
  )
}
