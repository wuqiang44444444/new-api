import { useEffect, useState } from 'react'

import { getSelf } from '@/lib/api'
import { useAuthStore, type AuthUser } from '@/stores/auth-store'

const MIN_REFRESH_INTERVAL_MS = 60_000

/** Refresh the cumulative projection without accepting another session's result. */
export function useUserProfileRefresh(): { refreshFailed: boolean } {
  const userId = useAuthStore((state) => state.auth.user?.id ?? null)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  const [refreshFailed, setRefreshFailed] = useState(false)

  useEffect(() => {
    // Each identity/session owns its request state. An old promise's finally
    // cannot clear the new session's in-flight flag.
    let active = true
    let inFlight = false
    let lastAttemptAt: number | null = null
    setRefreshFailed(false)
    if (!userId) return

    const isCurrentSession = () => {
      const auth = useAuthStore.getState().auth
      return (
        active &&
        auth.user?.id === userId &&
        (auth.session?.sid ?? null) === sessionId
      )
    }
    const refresh = async () => {
      if (!isCurrentSession() || inFlight) return
      const now = Date.now()
      if (
        lastAttemptAt !== null &&
        now - lastAttemptAt < MIN_REFRESH_INTERVAL_MS
      ) {
        return
      }
      lastAttemptAt = now
      inFlight = true
      try {
        const response = await getSelf()
        if (!isCurrentSession()) return
        const next = response?.data as AuthUser | undefined
        if (!next || next.id !== userId) {
          setRefreshFailed(true)
          return
        }
        useAuthStore.getState().auth.setUser(next)
        setRefreshFailed(false)
      } catch {
        if (isCurrentSession()) setRefreshFailed(true)
      } finally {
        inFlight = false
      }
    }

    void refresh()
    const onWindowFocus = () => void refresh()
    const onVisibilityChange = () => {
      if (document.visibilityState === 'visible') void refresh()
    }
    window.addEventListener('focus', onWindowFocus)
    document.addEventListener('visibilitychange', onVisibilityChange)
    const interval = window.setInterval(onWindowFocus, MIN_REFRESH_INTERVAL_MS)
    return () => {
      active = false
      window.removeEventListener('focus', onWindowFocus)
      document.removeEventListener('visibilitychange', onVisibilityChange)
      window.clearInterval(interval)
    }
  }, [userId, sessionId])

  return { refreshFailed }
}
