import { expect, test } from 'vitest'

import { displayValue } from './displayValue.ts'

test('shows a string as it is', () => {
  const shown = displayValue('Berlin')

  expect(shown).toBe('Berlin')
})

test('shows a richer value as compact JSON', () => {
  const shown = displayValue({ plan: 'pro', seats: [1, 2], active: true })

  expect(shown).toBe('{"plan":"pro","seats":[1,2],"active":true}')
})

test('shows a number as its digits', () => {
  const shown = displayValue(42)

  expect(shown).toBe('42')
})

test('has nothing to show for an empty string', () => {
  const shown = displayValue('')

  expect(shown).toBeUndefined()
})

test('has nothing to show for null', () => {
  const shown = displayValue(null)

  expect(shown).toBeUndefined()
})
