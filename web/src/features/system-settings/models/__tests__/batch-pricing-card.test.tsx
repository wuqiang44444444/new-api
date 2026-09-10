import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'

import { BatchPricingCard } from '../batch-pricing-card'
const { mutate } = vi.hoisted(() => ({ mutate: vi.fn() }))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
vi.mock('../../hooks/use-update-option', () => ({
  useUpdateOption: () => ({ mutate, isPending: false }),
}))
beforeEach(() => mutate.mockReset())
it('rejects invalid pricing maps and saves explicit Batch expressions', () => {
  render(<BatchPricingCard value='{}' />)
  const input = screen.getByLabelText('Batch billing expressions')
  const save = screen.getByRole('button', { name: 'Save' })
  fireEvent.change(input, { target: { value: '[]' } })
  expect(save).toBeDisabled()
  fireEvent.change(input, { target: { value: '{"model":"p * 2 + c * 4"}' } })
  fireEvent.click(save)
  expect(mutate).toHaveBeenCalledWith({
    key: 'batch_billing_setting.batch_billing_expr',
    value: '{"model":"p * 2 + c * 4"}',
  })
})
