import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import { expect, it, vi } from 'vitest'

import { ContractPricingSelect } from '../contract-pricing-select'
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
vi.mock('@/stores/auth-store', () => ({
  useAuthStore: (selector: (state: unknown) => unknown) =>
    selector({ auth: { user: { id: 7 } } }),
}))
vi.mock('@/features/keys/api', () => ({
  getSelfCustomerContract: async () => ({
    success: true,
    data: {
      contracts: [
        { id: 5, name: 'Contract A', enabled: true },
        { id: 6, name: 'Contract B', enabled: true },
      ],
    },
  }),
}))
it('starts with native pricing and requires an explicit contract selection', async () => {
  const onChange = vi.fn()
  render(
    <QueryClientProvider client={new QueryClient()}>
      <ContractPricingSelect value={null} onChange={onChange} />
    </QueryClientProvider>
  )
  await screen.findByRole('option', { name: 'Contract B' })
  const select = screen.getByLabelText('Pricing scope')
  expect(select).toHaveValue('')
  expect(onChange).not.toHaveBeenCalled()
  fireEvent.change(select, { target: { value: '6' } })
  expect(onChange).toHaveBeenCalledWith(6)
  fireEvent.change(select, { target: { value: 'batch' } })
  expect(onChange).toHaveBeenCalledWith('batch')
})
