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
import assert from 'node:assert/strict'
import { after, describe, test } from 'node:test'

import { Window } from 'happy-dom'

const domWindow = new Window()
const domGlobals = [
  'window',
  'document',
  'navigator',
  'HTMLElement',
  'HTMLInputElement',
  'SVGElement',
  'Node',
  'Element',
  'Event',
  'CustomEvent',
  'MutationObserver',
  'requestAnimationFrame',
  'cancelAnimationFrame',
  'getComputedStyle',
] as const

for (const key of domGlobals) {
  Object.defineProperty(globalThis, key, {
    configurable: true,
    value: domWindow[key],
  })
}

const { act, useState } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { GroupRatioVisualEditor } = await import('../group-ratio-visual-editor')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

type PricingFields = {
  GroupRatio: string
  UserUsableGroups: string
  GroupDescriptions: string
  TopupGroupRatio: string
}

function findPricingRow(container: HTMLElement, groupName: string) {
  const nameInput = [...container.querySelectorAll('input')].find(
    (input) => input.value === groupName
  )
  assert.ok(nameInput)
  const row = nameInput.closest('tr')
  assert.ok(row)
  return row
}

function PricingEditorHarness(props: {
  initialFields: PricingFields
  onFieldsChange?: (fields: PricingFields) => void
}) {
  const [fields, setFields] = useState(props.initialFields)

  return (
    <I18nextProvider i18n={i18n}>
      <GroupRatioVisualEditor
        groupRatio={fields.GroupRatio}
        userUsableGroups={fields.UserUsableGroups}
        groupDescriptions={fields.GroupDescriptions}
        topupGroupRatio={fields.TopupGroupRatio}
        groupGroupRatio='{}'
        autoGroups='[]'
        maxTokenAutoGroupsField={<div />}
        groupSpecialUsableGroup='{}'
        onChange={(field, value) => {
          setFields((current) => {
            const next = { ...current, [field]: value }
            props.onFieldsChange?.(next)
            return next
          })
        }}
      />
    </I18nextProvider>
  )
}

describe('pricing group descriptions', () => {
  after(() => {
    domWindow.close()
  })

  test('shows a persisted description when the group is not user selectable', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)

    await act(async () => {
      root.render(
        <PricingEditorHarness
          initialFields={{
            GroupRatio: '{"internal":1}',
            UserUsableGroups: '{}',
            GroupDescriptions: '{"internal":"Internal routing group"}',
            TopupGroupRatio: '{}',
          }}
        />
      )
    })

    const row = findPricingRow(container, 'internal')
    const descriptionInput = row.querySelector<HTMLInputElement>(
      'input[placeholder="Group description"]'
    )
    const selectable = row.querySelector<HTMLElement>(
      '[aria-label="User selectable"]'
    )

    assert.ok(descriptionInput)
    assert.equal(descriptionInput.value, 'Internal routing group')
    assert.ok(selectable)
    assert.equal(selectable.getAttribute('aria-checked'), 'false')

    await act(async () => root.unmount())
    container.remove()
  })

  test('keeps the description when user selectable is turned off', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    let latestFields: PricingFields | undefined

    await act(async () => {
      root.render(
        <PricingEditorHarness
          initialFields={{
            GroupRatio: '{"default":1}',
            UserUsableGroups: '{"default":"Default group"}',
            GroupDescriptions: '{}',
            TopupGroupRatio: '{}',
          }}
          onFieldsChange={(fields) => {
            latestFields = fields
          }}
        />
      )
    })

    const row = findPricingRow(container, 'default')
    const selectable = row.querySelector<HTMLElement>(
      '[aria-label="User selectable"]'
    )
    assert.ok(selectable)

    await act(async () => selectable.click())

    const descriptionInput = row.querySelector<HTMLInputElement>(
      'input[placeholder="Group description"]'
    )
    assert.ok(descriptionInput)
    assert.equal(descriptionInput.value, 'Default group')
    assert.ok(latestFields)
    assert.deepEqual(JSON.parse(latestFields.UserUsableGroups), {})
    assert.deepEqual(JSON.parse(latestFields.GroupDescriptions), {
      default: 'Default group',
    })

    await act(async () => root.unmount())
    container.remove()
  })
})
