import { expect, test } from 'vitest'

import {
  actorValue,
  auditFilterSchema,
  auditQuery,
  filterActorValue,
  filterChips,
  withAction,
  withActor,
  withPeriod,
  withValue,
} from './auditFilter.ts'

test('maps an empty filter to an empty query', () => {
  expect(auditQuery({})).toEqual({})
})

test('turns the period days into an inclusive start and an exclusive end', () => {
  const query = auditQuery({ from: '2026-03-01', to: '2026-03-31' })
  expect(query.from).toBe(new Date(2026, 2, 1).toISOString())
  expect(query.to).toBe(new Date(2026, 3, 1).toISOString())
})

test('a day missing its month and day starts on the first of the year', () => {
  expect(auditQuery({ from: '2026' }).from).toBe(new Date(2026, 0, 1).toISOString())
})

test('passes every other field through', () => {
  const filter = {
    actorKind: 'user' as const,
    actorId: '1',
    action: 'tag.create' as const,
    targetType: 'tag',
    targetId: '2',
    ip: '10.0.0.1',
    requestId: 'req-1',
  }
  expect(auditQuery(filter)).toEqual(filter)
})

test('parses a route search, coercing ids and dropping invalid values', () => {
  expect(
    auditFilterSchema.parse({
      targetId: 5,
      actorKind: 'robot',
      action: 'membership.transfer',
      ip: '1.2.3.4',
    }),
  ).toEqual({
    targetId: '5',
    actorKind: undefined,
    action: undefined,
    ip: '1.2.3.4',
  })
})

test('an action value narrows to its action; a cleared or unknown one narrows to none', () => {
  const before = { action: 'tag.create' as const, ip: '10.0.0.1' }
  expect(withAction(before, 'tag.delete')).toEqual({ action: 'tag.delete', ip: '10.0.0.1' })
  expect(withAction(before, null)).toEqual({ ip: '10.0.0.1' })
  expect(withAction(before, 'tag.rename')).toEqual({ ip: '10.0.0.1' })
})

test('an actor value carries its id only when it has one', () => {
  expect(actorValue({ kind: 'user', id: '2' })).toBe('user:2')
  expect(actorValue({ kind: 'system', id: null })).toBe('system')
  expect(filterActorValue({ actorKind: 'api_token', actorId: '77' })).toBe('api_token:77')
  expect(filterActorValue({ actorId: '77' })).toBeNull()
})

test('a "By" value narrows to its actor and replaces the one before', () => {
  const before = { action: 'tag.create' as const, actorKind: 'user' as const, actorId: '2' }
  expect(withActor(before, 'api_token:77')).toEqual({
    action: 'tag.create',
    actorKind: 'api_token',
    actorId: '77',
  })
  expect(withActor(before, 'system')).toEqual({ action: 'tag.create', actorKind: 'system' })
})

test('a cleared or unreadable "By" value narrows to no actor', () => {
  const before = { actorKind: 'user' as const, actorId: '2' }
  expect(withActor(before, null)).toEqual({})
  expect(withActor(before, 'robot:1')).toEqual({})
})

test('a period is applied only once both of its days are picked', () => {
  const before = { action: 'tag.create' as const, from: '2026-01-01', to: '2026-01-31' }
  expect(withPeriod(before, ['2026-03-01', '2026-03-31'])).toEqual({
    action: 'tag.create',
    from: '2026-03-01',
    to: '2026-03-31',
  })
  expect(withPeriod(before, ['2026-03-01', null])).toBeUndefined()
  expect(withPeriod(before, [null, null])).toEqual({ action: 'tag.create' })
})

test('chips show the single-value filters no select shows', () => {
  expect(
    filterChips({ targetType: 'integration', targetId: '5', ip: '10.0.0.1', requestId: 'req-a' }),
  ).toEqual([
    { field: 'targetId', value: '5' },
    { field: 'ip', value: '10.0.0.1' },
    { field: 'requestId', value: 'req-a' },
  ])
  expect(filterChips({ actorKind: 'user', actorId: '2' })).toEqual([])
  expect(filterChips({ actorId: '2' })).toEqual([{ field: 'actorId', value: '2' }])
})

test('a value narrows by its field, and an empty one stops narrowing by it', () => {
  expect(withValue({ ip: '10.0.0.1' }, 'requestId', 'req-a')).toEqual({
    ip: '10.0.0.1',
    requestId: 'req-a',
  })
  expect(withValue({ ip: '10.0.0.1', requestId: 'req-a' }, 'ip', null)).toEqual({
    requestId: 'req-a',
  })
})
