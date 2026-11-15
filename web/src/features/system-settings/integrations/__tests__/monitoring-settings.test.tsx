import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { MonitoringSettingsSection } from '../monitoring-settings-section'

test('wallet reminder shows quota units and updates its USD equivalent as the threshold changes', () => {
  const client = new QueryClient()
  render(
    <QueryClientProvider client={client}>
      <SettingsPageProvider actionsContainer={null}>
        <MonitoringSettingsSection
          defaultValues={{
            QuotaRemindThreshold: '1000',
            'perf_metrics_setting.enabled': true,
            'perf_metrics_setting.flush_interval': 5,
            'perf_metrics_setting.bucket_time': 'hour',
            'perf_metrics_setting.retention_days': 0,
          }}
        />
      </SettingsPageProvider>
    </QueryClientProvider>
  )

  const threshold = screen.getByRole('spinbutton', {
    name: 'Wallet reminder threshold (quota units)',
  })
  expect(threshold).toHaveValue(1000)
  expect(screen.getByText('Equivalent in USD: $0.002')).toBeVisible()
  fireEvent.change(threshold, { target: { value: '250000' } })
  expect(screen.getByText('Equivalent in USD: $0.5')).toBeVisible()
})

test('scientific notation and blank input have no preview and cannot be saved', async () => {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <SettingsPageProvider actionsContainer={null}>
        <MonitoringSettingsSection
          defaultValues={{
            QuotaRemindThreshold: '1000',
            'perf_metrics_setting.enabled': true,
            'perf_metrics_setting.flush_interval': 5,
            'perf_metrics_setting.bucket_time': 'hour',
            'perf_metrics_setting.retention_days': 0,
          }}
        />
      </SettingsPageProvider>
    </QueryClientProvider>
  )
  const threshold = screen.getByRole('spinbutton', {
    name: 'Wallet reminder threshold (quota units)',
  })
  const form = threshold.closest('form')
  if (!form) throw new Error('Wallet reminder form is missing')
  for (const value of ['1e4', '']) {
    fireEvent.change(threshold, { target: { value } })
    expect(screen.queryByText(/Equivalent in USD:/)).not.toBeInTheDocument()
    fireEvent.submit(form)
    expect(
      await screen.findByText(
        'Enter a whole number from 0 to 9,007,199,254,740,991'
      )
    ).toBeVisible()
  }
  fireEvent.change(threshold, { target: { value: '0' } })
  expect(screen.getByText('Equivalent in USD: $0')).toBeVisible()
})
