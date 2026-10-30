import { expect, test } from 'vitest'

import { draftState } from './draftState.ts'

test('a draft above a live version is pending, and can be published', () => {
  expect(draftState({ active: true, hasLive: true, hasDraft: true, dirty: false })).toEqual({
    pending: true,
    publishable: true,
  })
})

test('a never-published draft is not pending, and can be published', () => {
  expect(draftState({ active: false, hasLive: false, hasDraft: true, dirty: false })).toEqual({
    pending: false,
    publishable: true,
  })
})

test('with no draft, Publish resumes an inactive live version', () => {
  expect(draftState({ active: false, hasLive: true, hasDraft: false, dirty: false })).toEqual({
    pending: false,
    publishable: true,
  })
})

test('with no draft, an active one has nothing to publish', () => {
  expect(draftState({ active: true, hasLive: true, hasDraft: false, dirty: false })).toEqual({
    pending: false,
    publishable: false,
  })
})

test('Publish waits while the editor has unsaved changes', () => {
  expect(draftState({ active: true, hasLive: true, hasDraft: true, dirty: true })).toEqual({
    pending: true,
    publishable: false,
  })
})
