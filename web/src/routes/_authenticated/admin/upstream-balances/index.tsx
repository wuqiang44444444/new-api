import { createFileRoute, redirect } from '@tanstack/react-router'

import { UpstreamBalances } from '@/features/upstream-balances'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

export const Route = createFileRoute(
  '/_authenticated/admin/upstream-balances/'
)({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()
    if (!auth.user || auth.user.role < ROLE.ADMIN) {
      throw redirect({ to: '/403' })
    }
  },
  component: UpstreamBalances,
})
