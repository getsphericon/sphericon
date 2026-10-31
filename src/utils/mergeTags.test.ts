import { expect, test } from 'vitest'

import { eventPropertyTag } from './mergeTags.ts'

test('names a plain property of the triggering Event with a dot', () => {
  expect(eventPropertyTag(' amount ')).toBe('event.properties.amount')
})

test('names a property whose name is no plain word in brackets', () => {
  expect(eventPropertyTag('card hint')).toBe('event.properties["card hint"]')
  expect(eventPropertyTag('plan-id')).toBe('event.properties["plan-id"]')
})

test('names no property for a blank name or one a tag cannot quote', () => {
  expect(eventPropertyTag('  ')).toBeNull()
  expect(eventPropertyTag('say "hi"')).toBeNull()
  expect(eventPropertyTag('a}}b')).toBeNull()
})
