import { render, screen, within } from '@testing-library/react'
import { expect, it } from 'vitest'

import { formatCustomerStatementQuota } from '@/features/billing-reconciliation/lib'

import { PersonalUsageOverview } from '../personal-usage-overview'

it('shows only frozen pricing meters in the upstream overview', () => {
  render(
    <PersonalUsageOverview
      upstream
      metrics={{
        total_calls: 3,
        success_calls: 3,
        failure_calls: 0,
        cancelled_calls: 0,
        other_result_calls: 0,
        input_tokens: 90,
        output_tokens: 30,
        cache_read_tokens: 0,
        cache_write_tokens: 0,
        image_count: 8,
        image_count_missing_rows: 1,
        billable_image_missing_rows: 1,
        test_billable_images: 2,
        seconds: '6',
        gross_quota: 0,
        refund_quota: 0,
        net_quota: 0,
      }}
    />
  )

  expect(screen.getByRole('heading', { name: 'Billable tokens' })).toBeVisible()
  expect(screen.getByText('120')).toBeVisible()
  expect(screen.getByText('unrecorded')).toBeVisible()
  expect(
    screen.getByRole('heading', { name: 'Billable image count' })
  ).toBeVisible()
  expect(
    screen.getByRole('heading', { name: 'Channel test billable images' })
  ).toBeVisible()
  expect(screen.getByRole('heading', { name: 'Billing seconds' })).toBeVisible()
  expect(
    screen.queryByRole('heading', { name: 'Images generated' })
  ).not.toBeInTheDocument()
})

it('groups token and non-token metrics and highlights failed calls in the overview', () => {
  render(
    <PersonalUsageOverview
      metrics={{
        total_calls: 9_510,
        success_calls: 9_377,
        failure_calls: 112,
        cancelled_calls: 21,
        other_result_calls: 0,
        input_tokens: 218_897_319,
        output_tokens: 13_563_734,
        cache_read_tokens: 160_774_720,
        cache_write_tokens: 5_748_696,
        image_count: 188,
        seconds: '6',
        seconds_missing_rows: 1,
        rows_missing_tokens: 1,
        rows_missing_money: 1,
        gross_quota: 249_932_458,
        refund_quota: 0,
        net_quota: 249_932_458,
      }}
    />
  )

  const tokenCard = screen
    .getByRole('heading', { name: 'Total tokens' })
    .closest('[data-slot="card"]') as HTMLElement
  expect(within(tokenCard).getByText('232,461,053')).toBeVisible()
  expect(within(tokenCard).getByText('218,897,319')).toHaveClass(
    'whitespace-nowrap'
  )
  expect(within(tokenCard).getByText('13,563,734')).toBeVisible()
  expect(within(tokenCard).getByText('160,774,720')).toBeVisible()
  expect(within(tokenCard).getByText('5,748,696')).toBeVisible()
  expect(
    screen.getByText(formatCustomerStatementQuota(249_932_458))
  ).toHaveClass('whitespace-nowrap')
  const successCount = screen.getByText('OK 9,377')
  expect(successCount).toHaveClass('whitespace-nowrap')
  expect(successCount.parentElement).toHaveClass('flex-wrap')
  expect(screen.getByText('Failed 112')).toHaveClass('text-destructive')
  expect(screen.getByText('Cancelled 21')).toBeVisible()

  const nonTokenCard = screen
    .getByRole('heading', { name: 'Images generated' })
    .closest('[data-slot="card"]') as HTMLElement
  expect(within(nonTokenCard).getByText('188')).toBeVisible()
  expect(
    within(nonTokenCard).getByRole('heading', { name: 'Billing seconds' })
  ).toBeVisible()
  expect(within(nonTokenCard).getByText('6')).toBeVisible()
  expect(screen.queryByText('Known subtotal')).not.toBeInTheDocument()
})
