import { expect, test } from 'vitest'

import en from '../locales/en/translation.json' with { type: 'json' }

function entries(node: unknown, path: string[] = []): [string, string][] {
  if (typeof node === 'string') {
    return [[path.join('.'), node]]
  }
  if (typeof node !== 'object' || node === null) {
    return []
  }
  return Object.entries(node).flatMap(([key, child]) => entries(child, [...path, key]))
}

// A quantity a string prints is `{{count, number}}` (#1022, ADR 0096): bare, it is written in the
// Instance's language whatever Regional format the User chose.
test('a count in a string is always written with the number format', () => {
  const bare = entries(en).filter(([, text]) => text.includes('{{count}}'))

  expect(bare.map(([key]) => key)).toEqual([])
})
