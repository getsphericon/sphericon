import { expect, test } from 'vitest'

import { awaitedActionTag, awaitedEvent, EVENT, propertyTag } from './mergeTags.ts'

test('names a plain property of the triggering Event with a dot', () => {
  expect(propertyTag(EVENT, ' amount ')).toBe('event.properties.amount')
})

test('names a property whose name is no plain word in brackets', () => {
  expect(propertyTag(EVENT, 'card hint')).toBe('event.properties["card hint"]')
  expect(propertyTag(EVENT, 'plan-id')).toBe('event.properties["plan-id"]')
})

test('names no property for a blank name or one a tag cannot quote', () => {
  expect(propertyTag(EVENT, '  ')).toBeNull()
  expect(propertyTag(EVENT, 'say "hi"')).toBeNull()
  expect(propertyTag(EVENT, 'a}}b')).toBeNull()
})

test('names the action and a property of the Event a Wait for event arrived on by its name', () => {
  expect(awaitedActionTag('purchase')).toBe('awaited.purchase.action')
  expect(propertyTag(awaitedEvent('purchase'), 'product')).toBe(
    'awaited.purchase.properties.product',
  )
  expect(propertyTag(awaitedEvent('purchase'), 'line item')).toBe(
    'awaited.purchase.properties["line item"]',
  )
})
