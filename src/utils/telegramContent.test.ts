import { expect, test } from 'vitest'

import { completeButtons, resolveTags, telegramLimit } from './telegramContent.ts'

test('the limit is the caption limit once there is an image, the text limit otherwise', () => {
  expect(telegramLimit('')).toBe(4096)
  expect(telegramLimit('   ')).toBe(4096)
  expect(telegramLimit('https://cdn.example.com/a.png')).toBe(1024)
})

test('merge tags resolve to the sample Contact, escaped; an unknown tag renders empty', () => {
  expect(resolveTags('Hi {{ first_name }}, {{event.action}}{{ nope }}!')).toBe(
    'Hi Alex, order_placed!',
  )
  expect(resolveTags('{{ unsubscribe_url }}')).toBe('https://example.com/e/u/…')
})

test('a half-typed button is dropped', () => {
  expect(
    completeButtons([
      { label: 'Open', url: 'https://example.com' },
      { label: '  ', url: 'https://example.com' },
      { label: 'No URL', url: '' },
    ]),
  ).toEqual([{ label: 'Open', url: 'https://example.com' }])
})
