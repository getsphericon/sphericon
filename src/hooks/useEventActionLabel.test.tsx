import { Select, TagsInput } from '@mantine/core'
import { expect, test } from 'vitest'

import { renderWithProviders } from '../test/renderWithProviders.tsx'
import { useEventActionFilter, useEventActionLabel } from './useEventActionLabel.ts'

const actions = ['email.opened', 'email.complained', 'purchase']

// A picker whose values are the codes, as the free-text action fields are.
function CodePicker() {
  const filter = useEventActionFilter()
  return <TagsInput label="Events" data={actions} filter={filter} />
}

// A picker whose options carry the words, as the Recent activity filter does.
function LabelPicker() {
  const filter = useEventActionFilter()
  const labelOf = useEventActionLabel()
  return (
    <Select
      label="Events"
      searchable
      data={actions.map((value) => ({ value, label: labelOf(value) }))}
      filter={filter}
    />
  )
}

test('a picker of codes finds a System event by its words', async () => {
  const screen = await renderWithProviders(<CodePicker />)

  await screen.getByRole('combobox', { name: 'Events' }).fill('spam')

  await expect.element(screen.getByRole('option', { name: 'email.complained' })).toBeVisible()
  await expect.element(screen.getByRole('option', { name: 'email.opened' })).not.toBeInTheDocument()
})

test('a picker of words finds a System event by its code', async () => {
  const screen = await renderWithProviders(<LabelPicker />)

  await screen.getByRole('combobox', { name: 'Events' }).fill('email.op')

  await expect.element(screen.getByRole('option', { name: 'Opened an email' })).toBeVisible()
  await expect.element(screen.getByRole('option', { name: 'purchase' })).not.toBeInTheDocument()
})

test("a customer's action is found by its name", async () => {
  const screen = await renderWithProviders(<CodePicker />)

  await screen.getByRole('combobox', { name: 'Events' }).fill('PURCH')

  await expect.element(screen.getByRole('option', { name: 'purchase' })).toBeVisible()
})
