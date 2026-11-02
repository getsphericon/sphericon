import { expect, test } from 'vitest'

import { changeFrom, formatChange, formatPointChange } from './change.ts'

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

test('a rate changes by the points between the two rates, not by a share of the earlier one', () => {
  const up = formatPointChange(0.3, 0.2)
  const down = formatPointChange(0.25, 0.5)
  const fromNone = formatPointChange(0.05, 0)
  const flat = formatPointChange(0.4, 0.4)
  const fraction = formatPointChange(0.256, 0.25)

  expect(up).toBe('+10')
  expect(down).toBe('-25')
  expect(fromNone).toBe('+5')
  expect(flat).toBe('0')
  expect(fraction).toBe('+0.6')
})
