import { expect, test } from 'vitest'

import { splitInlineCode } from './inlineCode.ts'

test('sets the backticked commands of a message apart from its prose', () => {
  expect(
    splitInlineCode('Run `sphericon migrate` (in development `mise run db:migrate`), then reload.'),
  ).toStrictEqual([
    { text: 'Run ', code: false },
    { text: 'sphericon migrate', code: true },
    { text: ' (in development ', code: false },
    { text: 'mise run db:migrate', code: true },
    { text: '), then reload.', code: false },
  ])
})

test('keeps a message without backticks as one span of prose', () => {
  expect(splitInlineCode('The page could not be loaded.')).toStrictEqual([
    { text: 'The page could not be loaded.', code: false },
  ])
})

test('keeps a backtick with no partner as prose', () => {
  expect(splitInlineCode('`a` and ` b')).toStrictEqual([
    { text: 'a', code: true },
    { text: ' and ` b', code: false },
  ])
})
