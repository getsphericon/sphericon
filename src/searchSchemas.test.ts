import { expect, test } from 'vitest'

import { httpUrlParam } from './searchSchemas.ts'

test('keeps an absolute http(s) link and drops every other value', () => {
  expect(httpUrlParam.parse('https://example.com/e/u/all-token')).toBe(
    'https://example.com/e/u/all-token',
  )
  expect(httpUrlParam.parse('javascript:alert(1)')).toBeUndefined()
  expect(httpUrlParam.parse('/relative/path')).toBeUndefined()
  expect(httpUrlParam.parse(undefined)).toBeUndefined()
})
