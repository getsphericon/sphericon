import { afterEach, expect, test, vi } from 'vitest'

import { daysBefore, daysFromNow, formatLongDate, formatRelative } from './datetime.ts'

afterEach(() => {
  vi.useRealTimers()
})

test('a moment in the past reads as how long ago it was', () => {
  expect(formatRelative(Date.now() - 5 * 60 * 1000, 'en')).toBe('5 minutes ago')
})

test('a long date is spelled out in the given locale', () => {
  expect(formatLongDate('2026-10-30T12:00:00', 'en')).toBe('October 30, 2026')
})

test('days before a day cross month and year ends', () => {
  expect(daysBefore('2026-03-01', 1)).toBe('2026-02-28')
  expect(daysBefore('2026-01-05', 29)).toBe('2025-12-07')
})

test('days from now land on the same time of day, the calendar days on', () => {
  vi.useFakeTimers({ now: new Date(2026, 0, 30, 9, 30) })
  expect(daysFromNow(30)).toBe(new Date(2026, 2, 1, 9, 30).toISOString())
})
