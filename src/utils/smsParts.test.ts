import { expect, test } from 'vitest'

import table from '../../internal/smsrender/testdata/parts.json' with { type: 'json' }
import { measureSms } from './smsParts.ts'

// The table the backend counter reads too (internal/smsrender): the two agree on every case.
const cases = table.cases.map((c) => ({
  ...c,
  text: c.text.map(([piece, times]) => String(piece).repeat(Number(times))).join(''),
}))

test.each(cases)('$name', ({ text, encoding, characters, parts }) => {
  expect(measureSms(text)).toEqual({ encoding, characters, parts })
})

test('one Cyrillic letter switches a GSM-7 text to UCS-2', () => {
  expect(measureSms('Your order is ready').encoding).toBe('GSM-7')
  expect(measureSms('Your order is ready, Борис').encoding).toBe('UCS-2')
})
