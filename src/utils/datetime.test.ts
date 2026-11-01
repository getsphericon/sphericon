import { expect, test } from 'vitest'

import { daysBefore, formatLongDate, formatRelative } from './datetime.ts'

test('a moment in the past reads as how long ago it was', () => {
  expect(formatRelative(Date.now() - 5 * 60 * 1000)).toBe('5 minutes ago')
})

test('a long date is spelled out in the instance locale', () => {
  expect(formatLongDate('2026-10-30T12:00:00')).toBe('October 30, 2026')
})

test('days before a day cross month and year ends', () => {
  expect(daysBefore('2026-03-01', 1)).toBe('2026-02-28')
  expect(daysBefore('2026-01-05', 29)).toBe('2025-12-07')
})
