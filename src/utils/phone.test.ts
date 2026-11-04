import { expect, test } from 'vitest'

import { formatPhone } from './phone.ts'

test('a stored E.164 phone is shown in the readable international format', () => {
  expect(formatPhone('+14155550100')).toBe('+1 415 555 0100')
  expect(formatPhone('+49301234567')).toBe('+49 30 1234567')
  expect(formatPhone('+79991234567')).toBe('+7 999 123 45 67')
})

test('a value that is no E.164 number is shown as it is', () => {
  expect(formatPhone('not a phone')).toBe('not a phone')
})
