import { expect, test } from 'vitest'

import { formatDateTime, formatLongDate, formatPercent } from './format.ts'

// The Staff console keeps the Instance's language ("en" here) and the browser's zone.
test('the Staff console writes dates and percentages in the Instance language', () => {
  expect(formatLongDate('2026-12-31T12:00:00')).toBe('December 31, 2026')
  expect(formatDateTime('2026-12-31T13:05:09')).toMatch(/^12\/31\/2026 1:05:09\sPM$/)
  expect(formatPercent(0.1234)).toBe('12.34%')
})
