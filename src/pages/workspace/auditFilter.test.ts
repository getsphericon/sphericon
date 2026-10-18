import { expect, test } from 'vitest'

import { auditFilterSchema, auditQuery } from './auditFilter.ts'

test('maps an empty filter to an empty query', () => {
  expect(auditQuery({})).toEqual({})
})

test('turns the period days into an inclusive start and an exclusive end', () => {
  const query = auditQuery({ from: '2026-03-01', to: '2026-03-31' })
  expect(query.from).toBe(new Date(2026, 2, 1).toISOString())
  expect(query.to).toBe(new Date(2026, 3, 1).toISOString())
})

test('passes every other field through', () => {
  const filter = {
    actorKind: 'user' as const,
    actorId: '1',
    action: 'tag.create',
    targetType: 'tag',
    targetId: '2',
    ip: '10.0.0.1',
    requestId: 'req-1',
  }
  expect(auditQuery(filter)).toEqual(filter)
})

test('parses a route search, coercing ids and dropping invalid values', () => {
  expect(auditFilterSchema.parse({ targetId: 5, actorKind: 'robot', ip: '1.2.3.4' })).toEqual({
    targetId: '5',
    actorKind: undefined,
    ip: '1.2.3.4',
  })
})
