import { describe, expect, test } from 'vitest'

import {
  getApiErrorMessage,
  getFieldError,
  getRateLimitWait,
  isForbiddenError,
  isRateLimitedError,
} from './apiErrors.ts'

describe('getApiErrorMessage', () => {
  test('prefers detail over every other field', () => {
    const error = { detail: 'detail', form: 'form', error: 'error', title: 'title' }
    expect(getApiErrorMessage(error, 'fallback')).toBe('detail')
  })

  test('falls through detail → form → error → title → fallback', () => {
    expect(getApiErrorMessage({ form: 'form', error: 'error', title: 'title' }, 'fallback')).toBe(
      'form',
    )
    expect(getApiErrorMessage({ error: 'error', title: 'title' }, 'fallback')).toBe('error')
    expect(getApiErrorMessage({ title: 'title' }, 'fallback')).toBe('title')
  })

  test('returns the fallback when error is null, undefined, or empty', () => {
    expect(getApiErrorMessage(null, 'fallback')).toBe('fallback')
    expect(getApiErrorMessage(undefined, 'fallback')).toBe('fallback')
    expect(getApiErrorMessage({}, 'fallback')).toBe('fallback')
  })
})

describe('isRateLimitedError', () => {
  test('is true only for a 429 problem body', () => {
    expect(isRateLimitedError({ status: 429, detail: 'rate limit exceeded' })).toBe(true)
    expect(isRateLimitedError({ status: 403 })).toBe(false)
    expect(isRateLimitedError(null)).toBe(false)
  })
})

describe('getRateLimitWait', () => {
  test('counts whole minutes (rounded up) from a minute and seconds below', () => {
    expect(getRateLimitWait({ status: 429, retryAfter: 3600 })).toEqual({
      unit: 'minutes',
      count: 60,
    })
    expect(getRateLimitWait({ status: 429, retryAfter: 61 })).toEqual({
      unit: 'minutes',
      count: 2,
    })
    expect(getRateLimitWait({ status: 429, retryAfter: 45 })).toEqual({
      unit: 'seconds',
      count: 45,
    })
  })

  test('is undefined without a 429 or without a wait', () => {
    expect(getRateLimitWait({ status: 429 })).toBeUndefined()
    expect(getRateLimitWait({ status: 403, retryAfter: 30 })).toBeUndefined()
    expect(getRateLimitWait(null)).toBeUndefined()
  })
})

describe('isForbiddenError', () => {
  test('is true only for a 403 problem body', () => {
    expect(isForbiddenError({ status: 403, detail: 'insufficient role' })).toBe(true)
    expect(isForbiddenError({ status: 400 })).toBe(false)
    expect(isForbiddenError({ detail: 'no status' })).toBe(false)
    expect(isForbiddenError(null)).toBe(false)
  })
})

describe('getFieldError', () => {
  test('is the first message a 422 lists under the field', () => {
    const error = { status: 422, errors: { 'signupItems.terms': ['accept the Terms', 'again'] } }
    expect(getFieldError(error, 'signupItems.terms')).toBe('accept the Terms')
  })

  test('is undefined for another field, another status or no problem', () => {
    const error = { status: 422, errors: { name: ['the name is required'] } }
    expect(getFieldError(error, 'signupItems.terms')).toBeUndefined()
    expect(getFieldError({ status: 409, errors: { name: ['taken'] } }, 'name')).toBeUndefined()
    expect(getFieldError(null, 'name')).toBeUndefined()
  })
})
