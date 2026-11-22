import type { RuleGroupType } from 'react-querybuilder'
import { expect, test } from 'vitest'

import { MomentOperator } from '../../generated/site/types.gen.ts'
import type {
  EventLeaf,
  RuleConditionInput,
  SegmentCondition,
  SegmentConditionInput,
} from '../../generated/site/types.gen.ts'
import {
  emptyGroup,
  eventCount,
  eventPresets,
  toCondition,
  toDraft,
  toQuery,
  withinLastOperator,
} from './condition.ts'

// Every leaf a Segment's Condition admits, nested and negated.
const segment: SegmentConditionInput = {
  kind: 'group',
  combinator: 'or',
  not: true,
  entries: [
    {
      kind: 'attribute',
      attribute: 'email',
      test: { kind: 'text', operator: 'contains', value: 'acme' },
    },
    { kind: 'attribute', attribute: 'city', test: { kind: 'presence', operator: 'notSet' } },
    { kind: 'birthday', test: { kind: 'withinLast', days: 7 } },
    { kind: 'tag', name: 'vip', operator: 'doesNotHave' },
    { kind: 'telegram', operator: 'has' },
    { kind: 'sms', operator: 'doesNotHave' },
    {
      kind: 'group',
      combinator: 'and',
      entries: [
        { kind: 'customField', key: 'plan', test: { kind: 'text', operator: 'eq', value: 'pro' } },
        { kind: 'customField', key: 'seats', test: { kind: 'number', operator: 'gte', value: 10 } },
        { kind: 'customField', key: 'renews_at', test: { kind: 'presence', operator: 'set' } },
        { kind: 'event', action: 'purchase', operator: 'atLeast', times: 3, withinDays: 7 },
      ],
    },
  ],
}

// A Rule's Condition adds the test of the triggering Event's property, its key a field of its own.
const rule: RuleConditionInput = {
  kind: 'group',
  combinator: 'and',
  entries: [
    {
      kind: 'property',
      of: { kind: 'trigger' },
      key: 'url',
      test: { kind: 'text', operator: 'beginsWith', value: '/pricing' },
    },
    {
      kind: 'property',
      of: { kind: 'trigger' },
      key: 'order.total',
      test: { kind: 'number', operator: 'lt', value: 2.5 },
    },
    {
      kind: 'property',
      of: { kind: 'trigger' },
      key: 'coupon',
      test: { kind: 'presence', operator: 'set' },
    },
    { kind: 'customField', key: 'email', test: { kind: 'text', operator: 'neq', value: 'x' } },
    { kind: 'attribute', attribute: 'email', test: { kind: 'text', operator: 'neq', value: 'x' } },
  ],
}

test("a Segment's Condition survives the walk to the query builder and back as the same tree", () => {
  const back = toDraft(toQuery(segment))

  expect(back).toEqual(segment)
  expect(toCondition(back, 'segment')).toEqual(segment)
})

test("a Rule's Condition survives the walk too, a property's key and a Custom field named like an attribute kept apart", () => {
  const back = toDraft(toQuery(rule))

  expect(back).toEqual(rule)
  expect(toCondition(back, 'rule')).toEqual(rule)
})

test("a Segment's Condition is no Condition with a property test in it", () => {
  const back = toDraft(toQuery(rule))

  expect(toCondition(back, 'segment')).toBeUndefined()
})

test('a property without a key is no Condition yet', () => {
  const blank = toDraft({
    combinator: 'and',
    rules: [{ field: 'property', operator: 'eq', value: { key: '', value: 'x' } }],
  })

  expect(blank.entries).toEqual([
    {
      kind: 'property',
      of: { kind: 'trigger' },
      test: { kind: 'text', operator: 'eq', value: 'x' },
    },
  ])
  expect(toCondition(blank, 'rule')).toBeUndefined()
})

test('a property keyed before its value is typed keeps its key', () => {
  const keyed = toDraft({
    combinator: 'and',
    rules: [{ field: 'property', operator: 'set', value: { key: 'coupon' } }],
  })

  expect(keyed.entries).toEqual([
    {
      kind: 'property',
      of: { kind: 'trigger' },
      key: 'coupon',
      test: { kind: 'presence', operator: 'set' },
    },
  ])
})

test('the empty group becomes an empty query without a negation', () => {
  const query = toQuery(emptyGroup)

  expect(query).toEqual({ combinator: 'and', rules: [] })
})

test('the query builder tree is cleaned: an unknown operator is dropped, a bad combinator falls back to and', () => {
  const query: RuleGroupType = {
    combinator: 'xor',
    rules: [
      { field: 'email', operator: 'regex', value: '.*' },
      { field: 'email', operator: 'gt', value: 3 },
      { field: 'email', operator: 'eq', value: { a: 1 } },
    ],
  }

  const cleaned = toDraft(query)

  expect(cleaned).toEqual({
    kind: 'group',
    combinator: 'and',
    entries: [
      { kind: 'attribute', attribute: 'email', test: { kind: 'text', operator: 'eq', value: '' } },
    ],
  })
})

test("a number typed into a number Custom field's text test is its text", () => {
  const typed = toDraft({
    combinator: 'and',
    rules: [{ field: 'customField:seats', operator: 'eq', value: 12 }],
  })

  expect(typed.entries).toEqual([
    { kind: 'customField', key: 'seats', test: { kind: 'text', operator: 'eq', value: '12' } },
  ])
})

const one = (leaf: EventLeaf): SegmentCondition => ({
  kind: 'group',
  combinator: 'and',
  entries: [leaf],
})

const eventRule = (operator: string, value: unknown): RuleGroupType => ({
  combinator: 'and',
  rules: [{ field: 'event:purchase', operator, value }],
})

test('at least once and at most zero times show as the presets and come back as counts', () => {
  const performed = one({ kind: 'event', action: 'purchase', operator: 'atLeast', times: 1 })
  const notPerformed = one({
    kind: 'event',
    action: 'purchase',
    operator: 'atMost',
    times: 0,
    withinDays: 30,
  })

  const performedQuery = toQuery(performed)
  const notPerformedQuery = toQuery(notPerformed)

  expect(performedQuery.rules).toEqual([
    { field: 'event:purchase', operator: eventPresets.performed, value: {} },
  ])
  expect(notPerformedQuery.rules).toEqual([
    { field: 'event:purchase', operator: eventPresets.notPerformed, value: { withinDays: 30 } },
  ])
  expect(toDraft(performedQuery)).toEqual(performed)
  expect(toDraft(notPerformedQuery)).toEqual(notPerformed)
})

test('a count operator picked without a count keeps it blank, and the draft is no Condition yet', () => {
  const atLeast = toDraft(eventRule('atLeast', { withinDays: 7 }))
  const atMost = toDraft(eventRule('atMost', ''))
  const typed = toDraft(eventRule('atLeast', { times: 1 }))

  expect(atLeast.entries).toEqual([
    { kind: 'event', action: 'purchase', operator: 'atLeast', withinDays: 7 },
  ])
  expect(atMost.entries).toEqual([{ kind: 'event', action: 'purchase', operator: 'atMost' }])
  expect(toCondition(atLeast, 'segment')).toBeUndefined()
  expect(toCondition(atMost, 'segment')).toBeUndefined()
  expect(toCondition(typed, 'segment')).toEqual(
    one({ kind: 'event', action: 'purchase', operator: 'atLeast', times: 1 }),
  )
})

test('an event rule with a text operator is dropped, and a value out of range is blank', () => {
  const textOperator = toDraft(eventRule('eq', { times: 2 }))
  const negative = toDraft(eventRule('atMost', { times: -1 }))

  expect(textOperator.entries).toEqual([])
  expect(negative.entries).toEqual([{ kind: 'event', action: 'purchase', operator: 'atMost' }])
  expect(eventCount({ times: 2, withinDays: 0 })).toEqual({})
})

const compared = (operator: string, value: unknown): RuleGroupType => ({
  combinator: 'and',
  rules: [{ field: 'customField:seats', operator, value }],
})

test('a number typed as text counts, and a blank one keeps the draft from being a Condition', () => {
  const typed = toDraft(compared('lt', '2.5'))
  const blank = toDraft(compared('lt', ''))

  expect(typed.entries).toEqual([
    { kind: 'customField', key: 'seats', test: { kind: 'number', operator: 'lt', value: 2.5 } },
  ])
  expect(blank.entries).toEqual([
    { kind: 'customField', key: 'seats', test: { kind: 'number', operator: 'lt' } },
  ])
  expect(toCondition(blank, 'segment')).toBeUndefined()
})

test('a value that is no finite number leaves a number test blank', () => {
  const infinite = toDraft(compared('lt', Number.POSITIVE_INFINITY))
  const words = toDraft(compared('lt', 'a few'))

  expect(infinite.entries).toEqual([
    { kind: 'customField', key: 'seats', test: { kind: 'number', operator: 'lt' } },
  ])
  expect(toCondition(words, 'segment')).toBeUndefined()
})

// Paris is an hour ahead of UTC in March: a day there starts at 23:00 UTC the evening before.
const paris = 'Europe/Paris'

const dated = (operator: MomentOperator, at: string): SegmentCondition => ({
  kind: 'group',
  combinator: 'and',
  entries: [{ kind: 'customField', key: 'renews_at', test: { kind: 'moment', operator, at } }],
})

const picked = (operator: string, day: string): RuleGroupType => ({
  combinator: 'and',
  rules: [{ field: 'customField:renews_at', operator, value: day }],
})

test('before a day is before its start and after a day is from the next day, in the User zone', () => {
  const before = toDraft(picked('before', '2026-03-01'), paris)
  const after = toDraft(picked('after', '2026-03-01'), paris)

  expect(toCondition(before, 'segment')).toEqual(dated('before', '2026-02-28T23:00:00.000Z'))
  expect(toCondition(after, 'segment')).toEqual(dated('after', '2026-03-01T23:00:00.000Z'))
})

test('a loaded before or after shows the day it bounds, in the User zone', () => {
  const before = toQuery(dated('before', '2026-02-28T23:00:00Z'), paris)
  const after = toQuery(dated('after', '2026-03-01T23:00:00Z'), paris)
  const afterInUtc = toQuery(dated('after', '2026-03-01T23:00:00Z'), 'UTC')
  const notAMoment = toQuery(dated('before', 'not a moment'), paris)

  expect(before.rules).toEqual([
    { field: 'customField:renews_at', operator: 'before', value: '2026-03-01' },
  ])
  expect(after.rules).toEqual([
    { field: 'customField:renews_at', operator: 'after', value: '2026-03-01' },
  ])
  expect(afterInUtc.rules).toEqual([
    { field: 'customField:renews_at', operator: 'after', value: '2026-03-01' },
  ])
  expect(notAMoment.rules).toEqual([
    { field: 'customField:renews_at', operator: 'before', value: '' },
  ])
})

test('the birthday takes a moment or a window of days, and a blank one is no Condition yet', () => {
  const within = toDraft({
    combinator: 'and',
    rules: [{ field: 'birthday', operator: withinLastOperator, value: 7 }],
  })
  const noDays = toDraft({
    combinator: 'and',
    rules: [{ field: 'birthday', operator: withinLastOperator, value: '' }],
  })
  const noDay = toDraft({
    combinator: 'and',
    rules: [{ field: 'birthday', operator: 'before', value: '' }],
  })
  const notOnABirthday = toDraft({
    combinator: 'and',
    rules: [{ field: 'birthday', operator: 'eq', value: 'x' }],
  })

  expect(within.entries).toEqual([{ kind: 'birthday', test: { kind: 'withinLast', days: 7 } }])
  expect(noDays.entries).toEqual([{ kind: 'birthday', test: { kind: 'withinLast' } }])
  expect(noDay.entries).toEqual([
    { kind: 'birthday', test: { kind: 'moment', operator: 'before' } },
  ])
  expect(toCondition(noDays, 'segment')).toBeUndefined()
  expect(toCondition(noDay, 'segment')).toBeUndefined()
  expect(notOnABirthday.entries).toEqual([])
})

test('a combinator kept between the rules of the query builder is not a rule', () => {
  const kept = { field: 'customField:seats', operator: 'gte', value: 1 }
  const independent: RuleGroupType = { combinator: 'and', rules: [kept] }
  // react-querybuilder's independent combinators keep the combinator between the rules as text.
  Reflect.set(independent, 'rules', [kept, 'and'])

  expect(toDraft(independent).entries).toEqual([
    { kind: 'customField', key: 'seats', test: { kind: 'number', operator: 'gte', value: 1 } },
  ])
})

const alone = (field: string, operator: string, value: unknown): RuleGroupType => ({
  combinator: 'and',
  rules: [{ field, operator, value }],
})

test('a rule on a field that names no subject is dropped', () => {
  const unknownField = toDraft(alone('nickname', 'eq', 'Ada'))

  expect(unknownField.entries).toEqual([])
})

test('a Tag, Telegram or SMS rule is dropped unless it is had or not had', () => {
  const tagContains = toDraft(alone('tag', 'contains', 'vip'))
  const telegramSet = toDraft(alone('telegram', 'set', ''))
  const smsPerformed = toDraft(alone('sms', 'performed', ''))
  const tagHad = toDraft(alone('tag', 'has', 'vip'))

  expect(tagContains.entries).toEqual([])
  expect(telegramSet.entries).toEqual([])
  expect(smsPerformed.entries).toEqual([])
  expect(tagHad.entries).toEqual([{ kind: 'tag', name: 'vip', operator: 'has' }])
})

test('a property rule is dropped when its operator names no test a property takes, its key with it', () => {
  const before = toDraft(alone('property', 'before', { key: 'paid_at', value: '2026-03-01' }))
  const lastDays = toDraft(alone('property', withinLastOperator, { key: 'paid_at', value: 7 }))
  const counted = toDraft(alone('property', 'atLeast', { key: 'paid_at', value: { times: 1 } }))

  expect(before.entries).toEqual([])
  expect(lastDays.entries).toEqual([])
  expect(counted.entries).toEqual([])
})

test("a Custom field rule is dropped when its operator names no test, an event count's or a Tag's", () => {
  const performed = toDraft(alone('customField:plan', eventPresets.performed, ''))
  const has = toDraft(alone('customField:plan', 'has', 'pro'))

  expect(performed.entries).toEqual([])
  expect(has.entries).toEqual([])
})
