import { expect, test } from 'vitest'

import { formatPhone, isErasedNumber } from './phone.ts'

test('a stored E.164 phone is shown in the readable international format', () => {
  expect(formatPhone('+14155550100')).toBe('+1 415 555 0100')
  expect(formatPhone('+49301234567')).toBe('+49 30 1234567')
  expect(formatPhone('+79991234567')).toBe('+7 999 123 45 67')
})

test('a value that is no E.164 number is shown as it is', () => {
  expect(formatPhone('not a phone')).toBe('not a phone')
})

test('a number erased with its Contact is told by its hash, a number by its +', () => {
  expect(isErasedNumber('7a0b9e004fb40bf58219c1641ef73f0217e4494b0ec5c7de08e95cd7bb92561d')).toBe(
    true,
  )
  expect(isErasedNumber('+14155550100')).toBe(false)
  expect(isErasedNumber('alice@example.com')).toBe(false)
  expect(isErasedNumber('7A0B9E004FB40BF58219C1641EF73F0217E4494B0EC5C7DE08E95CD7BB92561D')).toBe(
    false,
  )
})
