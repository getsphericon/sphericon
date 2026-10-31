import { expect, test } from 'vitest'
import { renderHook } from 'vitest-browser-react'

import {
  fromLimit,
  NO_LIMITS,
  PER_DAY,
  PER_SECOND,
  requiresProviderLimit,
  toLimit,
  useProviderLimitValidators,
} from './providerLimit.ts'

test('an SES Integration with its own endpoint requires the Provider limit', () => {
  expect(requiresProviderLimit('ses', 'https://postbox.cloud.yandex.net')).toBe(true)
})

test('AWS SES, with no endpoint, reports its own limits', () => {
  expect(requiresProviderLimit('ses', null)).toBe(false)
  expect(requiresProviderLimit('ses', undefined)).toBe(false)
  expect(requiresProviderLimit('ses', '')).toBe(false)
  expect(requiresProviderLimit('ses', '   ')).toBe(false)
})

test('SMTP and the Test inbox never require it', () => {
  expect(requiresProviderLimit('smtp', 'https://postbox.cloud.yandex.net')).toBe(false)
  expect(requiresProviderLimit('inbox', null)).toBe(false)
})

test('a blank field is no limit, and a stored limit fills its field', () => {
  expect(toLimit('')).toBeNull()
  expect(toLimit('25')).toBe(25)
  expect(fromLimit(null)).toBe('')
  expect(fromLimit(25)).toBe(25)
})

test('a blank limit is fine unless the form requires one', async () => {
  const optional = await renderHook(() => useProviderLimitValidators())
  const required = await renderHook(() => useProviderLimitValidators(() => true))

  expect(optional.result.current.maxPerSecond('', NO_LIMITS)).toBeNull()
  expect(required.result.current.maxPerDay('', NO_LIMITS)).toBe(
    'Enter what your provider account allows',
  )
})

test('a limit must be a whole number within the contract bounds', async () => {
  const { result } = await renderHook(() => useProviderLimitValidators())

  expect(result.current.maxPerSecond(PER_SECOND.max, NO_LIMITS)).toBeNull()
  expect(result.current.maxPerSecond(PER_SECOND.max + 1, NO_LIMITS)).toBe(
    `Enter a whole number from ${PER_SECOND.min} to ${PER_SECOND.max}`,
  )
  expect(result.current.maxPerDay(1.5, NO_LIMITS)).toBe(
    `Enter a whole number from ${PER_DAY.min} to ${PER_DAY.max}`,
  )
})
