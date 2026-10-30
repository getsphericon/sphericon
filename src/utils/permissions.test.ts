import { expect, test } from 'vitest'

import { can } from './permissions.ts'

test('a Workspace allows the Permissions it lists', () => {
  expect(can({ permissions: ['webhooks:manage', 'brand_kit:manage'] }, 'brand_kit:manage')).toBe(
    true,
  )
})

test('a Workspace refuses a Permission it does not list', () => {
  expect(can({ permissions: ['webhooks:manage'] }, 'workspace:delete')).toBe(false)
})

test('a Workspace that is not loaded yet allows nothing', () => {
  expect(can(undefined, 'members:manage')).toBe(false)
})
