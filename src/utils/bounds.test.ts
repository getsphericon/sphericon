import { expect, test } from 'vitest'
import * as z from 'zod'

import { zMaxPerSecond } from '../generated/site/zod.gen.ts'
import { boundsOf } from './bounds.ts'

test("a contract number's range is its declared bounds", () => {
  expect(boundsOf(z.number().min(2).max(5))).toEqual({ min: 2, max: 5 })
  expect(boundsOf(zMaxPerSecond)).toEqual({
    min: zMaxPerSecond.minValue,
    max: zMaxPerSecond.maxValue,
  })
})

test('a number without a declared bound is refused', () => {
  expect(() => boundsOf(z.number().min(1))).toThrow('the contract declares no range')
  expect(() => boundsOf(z.number().max(1))).toThrow('the contract declares no range')
})
