import { expect, test } from 'vitest'

import { shortUrl } from './shortUrl.ts'

test('a link is shown without its scheme', () => {
  expect(shortUrl('https://hexlet.io/courses')).toBe('hexlet.io/courses')
  expect(shortUrl('http://hexlet.io/courses')).toBe('hexlet.io/courses')
})

test('a link without a scheme is left as it is', () => {
  expect(shortUrl('hexlet.io/courses')).toBe('hexlet.io/courses')
})

test('a long link is cut with an ellipsis to the length given', () => {
  const short = shortUrl(
    'https://hexlet.io/courses/python/lessons/loops-and-conditions?utm=mail',
    30,
  )
  expect(short).toHaveLength(30)
  expect(short.endsWith('…')).toBe(true)
  expect(short.startsWith('hexlet.io/courses/python')).toBe(true)
})

test('a link that fits is not cut', () => {
  expect(shortUrl('https://hexlet.io/a', 30)).toBe('hexlet.io/a')
})
