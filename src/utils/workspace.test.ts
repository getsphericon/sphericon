import { expect, test } from 'vitest'

import { siteWorkspacesListOptions } from '../generated/site/@tanstack/react-query.gen.ts'
import { workspace } from '../test/payloads.ts'
import { workspaceOptions } from './workspace.ts'

const acme = workspace({ id: 'acme', name: 'Acme' })
const globex = workspace({ id: 'globex', name: 'Globex' })

test('the Workspace is read from the cached Workspace list, not a query of its own', () => {
  const key = workspaceOptions('acme').queryKey
  expect(key).toEqual(siteWorkspacesListOptions().queryKey)
})

test('the Workspace of the id is picked from the list', () => {
  const picked = workspaceOptions('globex').select?.([acme, globex])
  expect(picked).toBe(globex)
})

test('a Workspace the list does not hold is undefined', () => {
  const picked = workspaceOptions('initech').select?.([acme, globex])
  expect(picked).toBeUndefined()
})
