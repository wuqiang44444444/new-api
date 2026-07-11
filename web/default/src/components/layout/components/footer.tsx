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

import { TokenAiMark } from '@/assets/token-ai-mark'
import { useStatus } from '@/hooks/use-status'
import { useSystemConfig } from '@/hooks/use-system-config'
import { DEFAULT_LOGO, DEFAULT_SYSTEM_NAME } from '@/lib/constants'
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

const UPSTREAM_REPO_URL = 'https://github.com/QuantumNous/new-api'

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

function FooterColumnView({ column }: { column: FooterColumn }) {
  return (
    <div>
      <h3 className='text-foreground text-sm font-semibold tracking-tight'>
        {column.title}
      </h3>
      <ul className='mt-3 space-y-2.5'>
        {column.links.map((link) => (
          <li key={`${link.href}-${link.text}`}>
            <a
              href={link.href}
              className='text-muted-foreground hover:text-foreground text-sm transition-colors duration-150'
            >
              {link.text}
            </a>
          </li>
        ))}
      </ul>
    </div>
  )
}

export function Footer(props: FooterProps) {
  const { t } = useTranslation()
  const { systemName, logo: systemLogo, footerHtml } = useSystemConfig()
  const displayLogo = systemLogo || props.logo || DEFAULT_LOGO
  const displayName = systemName || props.name || DEFAULT_SYSTEM_NAME
  const hasCustomLogo = !!displayLogo && displayLogo !== DEFAULT_LOGO
  const currentYear = new Date().getFullYear()

  const columns: FooterColumn[] = props.columns ?? [
    {
      title: t('footer.columns.about.title'),
      links: [
        {
          text: t('footer.columns.about.links.features'),
          href: '/#models',
        },
        {
          text: t('footer.columns.about.links.aboutProject'),
          href: '/about',
        },
        {
          text: t('footer.columns.about.links.contact'),
          href: '/#enterprise',
        },
      ],
    },
    {
      title: t('footer.columns.docs.title'),
      links: [
        {
          text: t('footer.columns.docs.links.quickStart'),
          href: '/#quick-start',
        },
        {
          text: t('footer.columns.docs.links.apiDocs'),
          href: '/#configuration',
        },
        {
          text: t('footer.columns.docs.links.installation'),
          href: '/#configuration',
        },
      ],
    },
  ]

  return (
    <footer
      id='enterprise-contact'
      className={cn('border-border bg-card border-t', props.className)}
    >
      <div className='mx-auto max-w-6xl px-4 py-10 sm:px-6'>
        <div className='flex flex-col gap-8 lg:flex-row lg:justify-between'>
          {/* Brand block */}
          <div className='max-w-sm'>
            <Link to='/' className='flex items-center gap-2.5'>
              <span className='text-primary flex size-7 items-center justify-center'>
                {hasCustomLogo ? (
                  <img
                    src={displayLogo}
                    alt={displayName}
                    className='size-7 rounded-md object-contain'
                  />
                ) : (
                  <TokenAiMark className='size-7' />
                )}
              </span>
              <span className='text-foreground text-lg font-semibold tracking-tight'>
                {displayName}
              </span>
            </Link>
            <p className='text-muted-foreground mt-3 text-sm leading-6'>
              {t(
                'Stable model access, simpler management, and transparent costs.'
              )}
            </p>
          </div>

          {/* Link columns */}
          <div className='grid grid-cols-2 gap-8 sm:gap-14'>
            {columns.map((column) => (
              <FooterColumnView key={column.title} column={column} />
            ))}
          </div>
        </div>

        {footerHtml && (
          <div
            className='custom-footer border-border text-muted-foreground mt-8 border-t pt-5 text-sm leading-6'
            // eslint-disable-next-line react/no-danger -- Footer HTML is an explicit administrator-managed site setting.
            dangerouslySetInnerHTML={{ __html: footerHtml }}
          />
        )}

        {/* Bottom bar: copyright + legal + upstream credit */}
        <div className='border-border text-muted-foreground mt-8 flex flex-col gap-3 border-t pt-5 text-xs sm:flex-row sm:items-center sm:justify-between'>
          <div className='flex flex-wrap items-center gap-x-2 gap-y-1'>
            <span>
              &copy; {currentYear} {displayName}.{' '}
              {props.copyright ?? t('footer.defaultCopyright')}
            </span>
            <LegalLinks />
          </div>
          <a
            href={UPSTREAM_REPO_URL}
            target='_blank'
            rel='noopener noreferrer'
            className='text-muted-foreground/80 hover:text-foreground transition-colors duration-150'
          >
            {t('footer.poweredBy', { name: 'new-api' })}
          </a>
        </div>
      </div>
    </footer>
  )
}
