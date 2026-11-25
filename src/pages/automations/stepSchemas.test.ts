import { expect, test } from 'vitest'

import { stepSchemas } from './stepSchemas.ts'

const HTTP = { stored: true, url: '', headers: [], body: '{}', secret: '' }

test('the HTTP step form takes a base64 signing secret, or none to keep the stored one', () => {
  const taken = ['', 'whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw', 'dGVzdC1zZWNyZXQ=']
  const refused = ['whsec_', 'whsec_x', 'whsec_not base64', 'submitted-secret']

  const takenRefused = taken.filter(
    (secret) => !stepSchemas.http.safeParse({ ...HTTP, secret }).success,
  )
  const refusedTaken = refused.filter(
    (secret) => stepSchemas.http.safeParse({ ...HTTP, secret }).success,
  )

  expect(takenRefused).toEqual([])
  expect(refusedTaken).toEqual([])
})

test('a new HTTP step needs its URL, a stored one keeps the stored URL when left empty', () => {
  const fresh = stepSchemas.http.safeParse({ ...HTTP, stored: false })
  const stored = stepSchemas.http.safeParse(HTTP)

  expect(fresh.error?.issues.map((issue) => [issue.path, issue.message])).toEqual([
    [['url'], 'This field is required'],
  ])
  expect(stored.success).toBe(true)
})

test('a wait lasts a whole number of seconds, at least one', () => {
  const results = [60, 0, 1.5, ''].map((seconds) => stepSchemas.wait.safeParse({ seconds }).success)

  expect(results).toEqual([true, false, false, false])
})

test('a tag step names a Tag', () => {
  const blank = stepSchemas.apply_tag.safeParse({ tag: '  ' })

  expect(blank.error?.issues.map((issue) => issue.message)).toEqual(['This field is required'])
})
