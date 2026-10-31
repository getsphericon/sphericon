import { expect, test } from 'vitest'

import { sendCapDays } from './sendCapDays.ts'

const fresh = { perDay: 5000, sentLast24h: 0 }

test('an audience that goes out within 24 hours of the Send cap has no estimate', () => {
  // A full bucket lets 5000 out at once and refills 5000 over the next 24 hours.
  expect(sendCapDays(5001, fresh)).toBeNull()
  expect(sendCapDays(10_000, fresh)).toBeNull()
  expect(sendCapDays(0, fresh)).toBeNull()
})

test('an audience beyond that goes out over the days its remainder takes at the Send cap', () => {
  expect(sendCapDays(10_001, fresh)).toBe(2)
  expect(sendCapDays(15_000, fresh)).toBe(2)
  expect(sendCapDays(15_001, fresh)).toBe(3)
})

test('the sends of the last 24 hours leave less to go out at once', () => {
  const busy = { perDay: 5000, sentLast24h: 4000 }

  expect(sendCapDays(6000, busy)).toBeNull()
  expect(sendCapDays(6001, busy)).toBe(2)
  expect(sendCapDays(20_000, busy)).toBe(4)
})

test('sends beyond the Send cap push the audience further out', () => {
  const overspent = { perDay: 5000, sentLast24h: 6000 }

  expect(sendCapDays(4000, overspent)).toBeNull()
  expect(sendCapDays(4001, overspent)).toBe(2)
})

test('without a Send cap there is no estimate', () => {
  expect(sendCapDays(1_000_000, null)).toBeNull()
  expect(sendCapDays(1_000_000, undefined)).toBeNull()
})
