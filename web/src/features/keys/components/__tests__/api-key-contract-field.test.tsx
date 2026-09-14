import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import { useForm } from 'react-hook-form'
import { beforeEach, expect, it, vi } from 'vitest'

import { Form } from '@/components/ui/form'

import { getApiKeyFormDefaultValues, type ApiKeyFormValues } from '../../lib'
import type { SelfContractSummary } from '../../types'
import { ApiKeyContractField } from '../api-key-contract-field'

const load = vi.fn()
vi.mock('../../api', () => ({ getSelfCustomerContract: () => load() }))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
const contracts: SelfContractSummary[] = [
  {
    id: 1,
    name: 'Alpha',
    enabled: true,
    version: 1,
    models: [{ model: 'alpha-model', discount: '0.8' }],
  },
  {
    id: 2,
    name: 'Beta',
    enabled: true,
    version: 1,
    models: [{ model: 'beta-model', discount: '0.5' }],
  },
]

function Fixture(props: { bound: number; isUpdate?: boolean }) {
  const form = useForm<ApiKeyFormValues>({
    defaultValues: {
      ...getApiKeyFormDefaultValues(false),
      contract_id: props.bound,
    },
  })
  return (
    <Form {...form}>
      <ApiKeyContractField
        form={form}
        open
        ready
        isUpdate={props.isUpdate ?? props.bound > 0}
      />
      <button type='button' onClick={() => form.setValue('contract_id', 2)}>
        Bind Beta
      </button>
    </Form>
  )
}
function show(bound = 0, isUpdate?: boolean) {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <Fixture bound={bound} isUpdate={isUpdate} />
    </QueryClientProvider>
  )
}
beforeEach(() =>
  load.mockReset().mockResolvedValue({ success: true, data: { contracts } })
)

it('hides the contract control when there are no contracts and no binding', async () => {
  load.mockResolvedValue({ success: true, data: { contracts: [] } })
  show()
  await vi.waitFor(() => expect(load).toHaveBeenCalled())
  expect(screen.queryByText('Customer contract')).toBeNull()
})

it.each([0, 1])(
  'shows business load failure without inventing binding state for key %s',
  async (bound) => {
    load.mockResolvedValue({ success: false, message: 'Unavailable' })
    show(bound)
    expect(
      await screen.findByText('Contract discount details failed to load')
    ).toBeTruthy()
    expect(screen.queryByText(/Disabled/)).toBeNull()
    expect(screen.queryByRole('combobox')).toBeNull()
  }
)

it('keeps an existing unbound key unbound when only one enabled contract exists', async () => {
  load.mockResolvedValue({ success: true, data: { contracts: [contracts[0]] } })
  show(0, true)
  expect(await screen.findByText('Do not bind a contract')).toBeTruthy()
  expect(screen.queryByText('alpha-model')).toBeNull()
})
it('updates the discount list when the binding changes', async () => {
  show(1)
  expect(await screen.findByText('alpha-model')).toBeTruthy()
  expect(screen.getByText('0.8')).toBeTruthy()
  fireEvent.click(screen.getByText('Bind Beta'))
  expect(await screen.findByText('beta-model')).toBeTruthy()
  expect(screen.getByText('0.5')).toBeTruthy()
  expect(screen.queryByText('alpha-model')).toBeNull()
})
it('keeps a disabled binding visible without presenting its prices as active', async () => {
  load.mockResolvedValue({
    success: true,
    data: { contracts: [{ ...contracts[0], enabled: false }] },
  })
  show(1)
  expect(await screen.findByText(/Alpha.*Disabled/)).toBeTruthy()
  expect(
    screen.getByText(
      'This contract is disabled. Its discounts are not in effect.'
    )
  ).toBeTruthy()
  expect(screen.queryByText('0.8')).toBeNull()
})
