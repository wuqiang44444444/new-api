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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, test, vi } from 'vitest'

import { TaskUsagePricingEditor } from '../task-usage-pricing-editor'

describe('task usage editor expression preservation', () => {
  test('keeps an unsupported existing expression when visual mode is selected', async () => {
    const user = userEvent.setup()
    const expression = 'tier("custom", u("tokens") * u("tokens") / 1000000)'
    const onChange = vi.fn()
    render(
      <TaskUsagePricingEditor
        billingExpr={expression}
        requestRuleExpr=''
        usageSchema={{ tokens: { type: 'number', unit: 'token' } }}
        onBillingExprChange={onChange}
        onRequestRuleExprChange={vi.fn()}
      />
    )

    await user.click(screen.getByRole('combobox'))
    await user.click(
      await screen.findByRole('option', { name: 'Visual editor' })
    )

    const expressionInput = screen.getByRole('textbox', { name: 'Expression' })
    expect(expressionInput).toHaveValue(expression)
    expect(onChange).not.toHaveBeenCalled()
    expect(
      screen.getByText(
        'This expression cannot be converted to the visual editor without changing its pricing. Continue editing the expression.'
      )
    ).toBeInTheDocument()
    await user.clear(expressionInput)
    expect(onChange).toHaveBeenLastCalledWith('')
    await user.click(screen.getByRole('combobox'))
    await user.click(
      await screen.findByRole('option', { name: 'Visual editor' })
    )
    expect(expressionInput).not.toBeInTheDocument()
    expect(onChange).toHaveBeenLastCalledWith(
      expect.stringContaining('u("tokens")')
    )
  })
})
