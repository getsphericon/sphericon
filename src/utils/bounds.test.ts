import { expect, test } from 'vitest'
import * as z from 'zod'

import { zMaxPerSecond } from '../generated/site/zod.gen.ts'
import { boundsOf, maxItemsOf } from './bounds.ts'

test("a contract number's range is its declared bounds", () => {
  expect(boundsOf(z.number().min(2).max(9))).toEqual({ min: 2, max: 9 })
  expect(boundsOf(zMaxPerSecond)).toEqual({
    min: zMaxPerSecond.minValue,
    max: zMaxPerSecond.maxValue,
  })
})

test('a number without a declared bound is refused', () => {
  expect(() => boundsOf(z.number().min(1))).toThrow('the contract declares no range')
  expect(() => boundsOf(z.number().max(1))).toThrow('the contract declares no range')
})

test("a contract list's most items is its declared bound, and a list without one is refused", () => {
  expect(maxItemsOf(z.array(z.string()).max(5))).toBe(5)
  expect(() => maxItemsOf(z.array(z.string()))).toThrow('the contract declares no most items')
})
