import { expect, test } from 'vitest'

import { formatDuration } from './duration.ts'

test('a duration shows its largest unit and the one below it', () => {
  const days = formatDuration(86_400 + 3 * 3_600 + 59, 'en')
  const hours = formatDuration(23 * 3_600 + 37 * 60 + 30, 'en')
  const minutes = formatDuration(20 * 60 + 5, 'en')

  expect(days).toBe('1d 3h')
  expect(hours).toBe('23h 37m')
  expect(minutes).toBe('20m 5s')
})

test('a duration leaves out a zero second unit', () => {
  const wholeDay = formatDuration(86_400, 'en')
  const seconds = formatDuration(45, 'en')
  const none = formatDuration(0, 'en')

  expect(wholeDay).toBe('1d')
  expect(seconds).toBe('45s')
  expect(none).toBe('0s')
})
