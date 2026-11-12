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
import { useNavigate } from '@tanstack/react-router'
import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { PasswordInput } from '@/components/password-input'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { api } from '@/lib/api'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import { AuthLayout } from '../auth-layout'

export function ResetPasswordConfirm() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [token] = useState(() =>
    new URLSearchParams(window.location.hash.slice(1)).get('token')
  )
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [loading, setLoading] = useState(false)
  const [succeeded, setSucceeded] = useState(false)

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!token || loading || succeeded) return
    if (password !== confirmPassword) {
      toast.error(t('Passwords do not match'))
      return
    }

    setLoading(true)
    try {
      const res = await api.post('/api/user/reset', {
        token,
        password,
        confirm_password: confirmPassword,
      })
      requireServerSuccess(res.data)
      setPassword('')
      setConfirmPassword('')
      setSucceeded(true)
      window.history.replaceState(
        window.history.state,
        '',
        window.location.pathname
      )
    } catch (error) {
      handleServerError(error)
    } finally {
      setLoading(false)
    }
  }

  return (
    <AuthLayout>
      <div className='w-full space-y-8'>
        <div className='space-y-2'>
          <h2 className='text-center text-2xl font-semibold tracking-tight sm:text-left'>
            {t('Reset password')}
          </h2>
          <p className='text-muted-foreground text-sm'>
            {succeeded
              ? t('Your password has been reset.')
              : t('Enter and confirm your new password.')}
          </p>
        </div>
        {!token && (
          <Alert variant='destructive'>
            <AlertDescription>
              {t('Invalid reset link, please request a new password reset.')}
            </AlertDescription>
          </Alert>
        )}
        {token && !succeeded && (
          <form onSubmit={handleSubmit} className='space-y-4'>
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor='reset-new-password'>
                  {t('New password')}
                </FieldLabel>
                <PasswordInput
                  id='reset-new-password'
                  autoComplete='new-password'
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  required
                  disabled={loading}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor='reset-confirm-password'>
                  {t('Confirm password')}
                </FieldLabel>
                <PasswordInput
                  id='reset-confirm-password'
                  autoComplete='new-password'
                  value={confirmPassword}
                  onChange={(event) => setConfirmPassword(event.target.value)}
                  required
                  disabled={loading}
                />
              </Field>
            </FieldGroup>
            <Button type='submit' className='w-full' disabled={loading}>
              {t('Reset password')}
            </Button>
          </form>
        )}
        <Button
          type='button'
          variant='link'
          className='w-full'
          onClick={() => navigate({ to: '/sign-in', replace: true })}
        >
          {t('Back to login')}
        </Button>
      </div>
    </AuthLayout>
  )
}
