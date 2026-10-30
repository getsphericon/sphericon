import { expect, test } from 'vitest'

import { SignupItemKind } from '../generated/site/types.gen.ts'
import type { SiteSignupItem } from '../generated/site/types.gen.ts'
import { SignupItemKind as StaffSignupItemKind } from '../generated/staff/types.gen.ts'
import { publishStopsUsers, signupItemAnswers } from './signupItems.ts'

const terms: SiteSignupItem = {
  key: 'terms',
  title: 'Terms of service',
  kind: SignupItemKind.REQUIRED_ACCEPTANCE,
  version: 'v2',
}
const news: SiteSignupItem = {
  key: 'news',
  title: 'News',
  kind: SignupItemKind.OPTIONAL_CONSENT,
  version: 'v1',
}
const notice: SiteSignupItem = {
  key: 'notice',
  title: 'Notice',
  kind: SignupItemKind.NOTICE,
  version: 'v1',
}

test('answers every item shown at its version, a box unticked unless the person ticked it', () => {
  const answers = signupItemAnswers([terms, news, notice], { terms: true })

  expect(answers).toEqual([
    { key: 'terms', version: 'v2', accepted: true },
    { key: 'news', version: 'v1', accepted: false },
    { key: 'notice', version: 'v1' },
  ])
})

test('sends nothing when the Instance has no Sign-up items', () => {
  expect(signupItemAnswers([], {})).toBeUndefined()
})

test('publishing stops Users only for a required acceptance that is in effect', () => {
  const required = { kind: StaffSignupItemKind.REQUIRED_ACCEPTANCE }

  expect(publishStopsUsers(required)).toBe(true)
  expect(publishStopsUsers({ ...required, archivedAt: '2026-10-20T00:00:00Z' })).toBe(false)
  expect(publishStopsUsers({ kind: StaffSignupItemKind.OPTIONAL_CONSENT })).toBe(false)
  expect(publishStopsUsers({ kind: StaffSignupItemKind.NOTICE })).toBe(false)
})
