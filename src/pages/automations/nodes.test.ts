import { expect, test } from 'vitest'

import { SIGNING_SECRET_PATTERN } from './nodes.ts'

const signingSecret = new RegExp(SIGNING_SECRET_PATTERN)

test('the HTTP step form takes a base64 signing secret, or none to keep the stored one', () => {
  const taken = ['', 'whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw', 'dGVzdC1zZWNyZXQ=']
  const refused = ['whsec_', 'whsec_x', 'whsec_not base64', 'submitted-secret']

  expect(taken.filter((secret) => !signingSecret.test(secret))).toEqual([])
  expect(refused.filter((secret) => signingSecret.test(secret))).toEqual([])
})
