import { expect, test } from 'vitest'

import type { SiteSendingRulesResource } from '../../../generated/site/types.gen.ts'
import { clock, newWindow, periodIn, sendingRulesSchema, toValues } from './sendingRules.ts'

const messages = {
  countryOffPhone: 'country off phone',
  capIncomplete: 'cap incomplete',
  neverOpens: 'never opens',
  closesAfterOpens: 'closes after opens',
  timeRequired: 'time required',
}
const schema = sendingRulesSchema(messages)

const defaults: SiteSendingRulesResource = {
  frequencyCaps: [{ channel: 'sms', country: null, maxMessages: 3, periodMinutes: 1440 }],
  sendingWindows: [
    {
      channel: 'sms',
      country: null,
      hours: [
        { day: 'monday', opensAt: 480, closesAt: 1200 },
        { day: 'saturday', opensAt: 600, closesAt: 1440 },
      ],
    },
  ],
  revision: 4,
}

test('the rules the form opens with are saved back unchanged', () => {
  const body = schema.parse(toValues(defaults))

  expect(body).toEqual({
    frequencyCaps: defaults.frequencyCaps,
    sendingWindows: defaults.sendingWindows,
  })
})

test('a window that closes at 00:00 closes at midnight at the end of the day', () => {
  const values = toValues(defaults)

  expect(values.sendingWindows[0]?.days.saturday).toEqual({
    open: true,
    opensAt: '10:00',
    closesAt: '00:00',
  })
  expect(values.sendingWindows[0]?.days.sunday.open).toBe(false)
})

test('a blank country is the channel own rule and a lower-case one is read in capitals', () => {
  const body = schema.parse({
    frequencyCaps: [
      { channel: 'sms', country: 'us', maxMessages: 1, periodMinutes: 60 },
      { channel: 'email', country: '', maxMessages: '', periodMinutes: '' },
    ],
    sendingWindows: [],
  })

  expect(body.frequencyCaps).toEqual([
    { channel: 'sms', country: 'US', maxMessages: 1, periodMinutes: 60 },
    { channel: 'email', country: null, maxMessages: null, periodMinutes: null },
  ])
})

test('a cap with a maximum and no period, or a country on email, is refused', () => {
  const result = schema.safeParse({
    frequencyCaps: [
      { channel: 'sms', country: '', maxMessages: 3, periodMinutes: '' },
      { channel: 'email', country: 'US', maxMessages: 3, periodMinutes: 60 },
    ],
    sendingWindows: [],
  })

  expect(result.error?.issues.map((issue) => [issue.path.join('.'), issue.message])).toEqual([
    ['frequencyCaps.0.periodMinutes', 'cap incomplete'],
    ['frequencyCaps.1.country', 'country off phone'],
  ])
})

test('a window that never opens, or closes before it opens, is refused', () => {
  const closed = newWindow()
  for (const day of Object.values(closed.days)) {
    day.open = false
  }
  const backwards = newWindow()
  backwards.days.monday = { open: true, opensAt: '20:00', closesAt: '08:00' }

  const result = schema.safeParse({ frequencyCaps: [], sendingWindows: [closed, backwards] })

  expect(result.error?.issues.map((issue) => [issue.path.join('.'), issue.message])).toEqual([
    ['sendingWindows.0.country', 'never opens'],
    ['sendingWindows.1.days.monday.closesAt', 'closes after opens'],
  ])
})

test('a period reads in the largest unit that divides it', () => {
  expect([periodIn(10080), periodIn(120), periodIn(90)]).toEqual([
    { unit: 'days', count: 7 },
    { unit: 'hours', count: 2 },
    { unit: 'minutes', count: 90 },
  ])
})

test('a minute of the day reads as a clock', () => {
  expect([clock(0), clock(510), clock(1440)]).toEqual(['00:00', '08:30', '24:00'])
})
