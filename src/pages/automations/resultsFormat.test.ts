import { expect, test } from 'vitest'

import { changeFrom, formatChange, formatDuration } from './resultsFormat.ts'

test('a change is a share of the previous period', () => {
  const grew = changeFrom(6, 4)
  const fell = changeFrom(1, 4)
  const gone = changeFrom(0, 4)

  expect(grew).toBe(0.5)
  expect(fell).toBe(-0.75)
  expect(gone).toBe(-1)
})

test('a rise from a negative figure is a rise', () => {
  const lessRefunded = changeFrom(-500, -1000)
  const intoProfit = changeFrom(1000, -1000)
  const moreRefunded = changeFrom(-2000, -1000)

  expect(lessRefunded).toBe(0.5)
  expect(intoProfit).toBe(2)
  expect(moreRefunded).toBe(-1)
})

test('a change from a period with none has no share', () => {
  const fromNone = changeFrom(3, 0)

  expect(fromNone).toBeNull()
})

test('a change is shown with its sign, and no change without one', () => {
  const up = formatChange(6, 4)
  const down = formatChange(4, 5)
  const flat = formatChange(4, 4)
  const fromNone = formatChange(4, 0)

  expect(up).toBe('+50%')
  expect(down).toBe('-20%')
  expect(flat).toBe('0%')
  expect(fromNone).toBeNull()
})

test('a duration shows its largest unit and the one below it', () => {
  const days = formatDuration(86_400 + 3 * 3_600 + 59)
  const hours = formatDuration(23 * 3_600 + 37 * 60 + 30)
  const minutes = formatDuration(20 * 60 + 5)

  expect(days).toBe('1d 3h')
  expect(hours).toBe('23h 37m')
  expect(minutes).toBe('20m 5s')
})

test('a duration leaves out a zero second unit', () => {
  const wholeDay = formatDuration(86_400)
  const seconds = formatDuration(45)
  const none = formatDuration(0)

  expect(wholeDay).toBe('1d')
  expect(seconds).toBe('45s')
  expect(none).toBe('0s')
})
