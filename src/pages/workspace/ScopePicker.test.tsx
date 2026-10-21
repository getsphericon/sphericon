import { useState } from 'react'
import { expect, test } from 'vitest'

import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { ScopePicker } from './ScopePicker.tsx'

const OPTIONS = ['contacts:read', 'contacts:write', 'events:read']

function Harness({ onValue }: { onValue: (value: string[]) => void }) {
  const [value, setValue] = useState<string[]>([])
  return (
    <ScopePicker
      label="Scopes"
      options={OPTIONS}
      value={value}
      onChange={(next) => {
        setValue(next)
        onValue(next)
      }}
    />
  )
}

test('a resource checkbox selects all of its actions and clears them again', async () => {
  let latest: string[] = []
  const { screen } = await renderWithRouter(
    <Harness
      onValue={(v) => {
        latest = v
      }}
    />,
  )

  await screen.getByRole('checkbox', { name: 'contacts', exact: true }).click()
  expect(latest).toEqual(['contacts:read', 'contacts:write'])

  await screen.getByRole('checkbox', { name: 'contacts', exact: true }).click()
  expect(latest).toEqual([])
})

test('one action leaves its resource partly selected', async () => {
  let latest: string[] = []
  const { screen } = await renderWithRouter(
    <Harness
      onValue={(v) => {
        latest = v
      }}
    />,
  )

  await screen.getByRole('checkbox', { name: 'write' }).click()

  expect(latest).toEqual(['contacts:write'])
  await expect
    .element(screen.getByRole('checkbox', { name: 'contacts', exact: true }))
    .toBePartiallyChecked()
})
