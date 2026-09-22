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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { VideoFundLogs } from '../video-funds'
import { ErrorLogViewer } from './components/error-log-viewer'

export function ErrorLogs() {
  const { t } = useTranslation()
  const [showFunds, setShowFunds] = useState(false)
  const user = useAuthStore((state) => state.auth.user)
  const canRead = !!user && user.role >= ROLE.ADMIN
  const viewer = showFunds ? <VideoFundLogs /> : <ErrorLogViewer />
  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>{t('Error Logs')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='flex h-full min-h-0 flex-col gap-3'>
          {canRead && (
            <div className='flex gap-2'>
              <Button
                variant={showFunds ? 'outline' : 'default'}
                onClick={() => setShowFunds(false)}
              >
                {t('Error Logs')}
              </Button>
              <Button
                variant={showFunds ? 'default' : 'outline'}
                onClick={() => setShowFunds(true)}
              >
                {t('Funds hold logs')}
              </Button>
            </div>
          )}
          <div className='flex min-h-0 flex-1 flex-col'>
            {canRead ? (
              viewer
            ) : (
              <p role='status' className='text-muted-foreground text-sm'>
                {t('You do not have permission to perform this action.')}
              </p>
            )}
          </div>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
