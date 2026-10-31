import { expect, test } from 'vitest'

import { requiresProviderLimit } from './providerLimit.ts'

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
