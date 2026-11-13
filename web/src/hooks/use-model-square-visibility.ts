import { useStatus } from '@/hooks/use-status'
import {
  canShowModelSquare,
  getModuleAccessFromStatus,
} from '@/lib/nav-modules'
import { useAuthStore } from '@/stores/auth-store'

export function useModelSquareVisibility() {
  const { status, statusConfirmed } = useStatus()
  const signedIn = useAuthStore((state) => !!state.auth.user)
  return (
    statusConfirmed &&
    canShowModelSquare(getModuleAccessFromStatus(status, 'pricing'), signedIn)
  )
}
