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
import { Fragment } from 'react'
import { useTranslation } from 'react-i18next'

import { useStatus } from '@/hooks/use-status'
import { useSystemConfig } from '@/hooks/use-system-config'
import { cn } from '@/lib/utils'

interface FooterLink {
  text: string
  href: string
}

interface FooterColumn {
  title: string
  links: FooterLink[]
}

interface FooterProps {
  logo?: string
  name?: string
  columns?: FooterColumn[]
  copyright?: string
  className?: string
}

const NEW_API_FOOTER_ATTRIBUTION_KEY = [
  'footer',
  'new' + 'api',
  'projectAttributionSuffix',
].join('.')

function LegalLinks() {
  const { t } = useTranslation()
  const { status } = useStatus()
  const items: { key: string; label: string; href: string }[] = []

  if (status?.user_agreement_enabled) {
    items.push({
      key: 'user-agreement',
      label: t('User Agreement'),
      href: '/user-agreement',
    })
  }
  if (status?.privacy_policy_enabled) {
    items.push({
      key: 'privacy-policy',
      label: t('Privacy Policy'),
      href: '/privacy-policy',
    })
  }

  if (items.length === 0) return null

  return items.map((item, index) => (
    <Fragment key={item.key}>
      {index > 0 && (
        <span aria-hidden='true' className='text-muted-foreground/40'>
          ·
        </span>
      )}
      <Link
        to={item.href}
        className='hover:text-foreground transition-colors duration-150'
      >
        {item.label}
      </Link>
    </Fragment>
  ))
}

export function Footer(props: FooterProps) {
  const { t } = useTranslation()
  const { systemName, logo: systemLogo, footerHtml } = useSystemConfig()
  const displayLogo = systemLogo || props.logo || '/logo.png'
  const displayName = systemName || props.name || 'New API'
  const currentYear = new Date().getFullYear()

  return (
    <footer
      id='enterprise-contact'
      className={cn('border-border bg-card border-t', props.className)}
    >
      <div className='mx-auto max-w-6xl px-4 py-8 sm:px-6'>
        <div className='flex flex-col justify-between gap-5 sm:flex-row sm:items-center'>
          <Link to='/' className='flex items-center gap-3'>
            <img
              src={displayLogo}
              alt={displayName}
              className='size-8 rounded-md object-contain'
            />
            <span className='text-lg font-semibold tracking-tight'>
              {displayName}
            </span>
          </Link>
          <p className='text-muted-foreground max-w-xl text-sm leading-6 sm:text-right'>
            {t(
              'Stable model access, simpler management, and transparent costs.'
            )}
          </p>
        </div>

        {footerHtml && (
          <div
            className='custom-footer border-border text-muted-foreground mt-6 border-t pt-5 text-sm leading-6'
            // eslint-disable-next-line react/no-danger -- Footer HTML is an explicit administrator-managed site setting.
            dangerouslySetInnerHTML={{ __html: footerHtml }}
          />
        )}

        <div className='border-border text-muted-foreground mt-6 flex flex-col gap-3 border-t pt-5 text-xs sm:flex-row sm:items-center sm:justify-between'>
          <div className='flex flex-wrap items-center gap-x-2 gap-y-1'>
            <span>
              &copy; {currentYear} {displayName}.{' '}
              {props.copyright ?? t('footer.defaultCopyright')}
            </span>
            <LegalLinks />
          </div>
          <span>
            <a
              href='https://github.com/QuantumNous/new-api'
              target='_blank'
              rel='noopener noreferrer'
              className='text-foreground/75 hover:text-foreground font-medium transition-colors duration-150'
            >
              {t('New API')}
            </a>{' '}
            {t(NEW_API_FOOTER_ATTRIBUTION_KEY)}
          </span>
        </div>
      </div>
    </footer>
  )
}
