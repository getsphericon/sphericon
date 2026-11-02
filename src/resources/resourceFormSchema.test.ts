import { expect, test } from 'vitest'

import { emptyGroup } from '../components/conditions/condition.ts'
import {
  zCreateTemplateInput,
  zSiteCreateRuleInput,
  zSiteUpdateContactInput,
} from '../generated/site/zod.gen.ts'
import { resourceFormSchema } from './resourceFormSchema.ts'

test('loaded collections stay typed and clearing them supplies an empty collection', () => {
  const form = resourceFormSchema(zSiteUpdateContactInput.pick({ firstName: true, tags: true }), {
    firstName: '',
    tags: [] as string[],
  })
  const loaded = form.toValues({ firstName: 'Ada', tags: ['vip'] })
  expect(loaded).toEqual({ firstName: 'Ada', tags: ['vip'] })

  const payload = form.update.parse({ ...loaded, tags: [] })
  expect(payload).toEqual({ firstName: 'Ada', tags: [] })
})

test('an absent update preserves a field, a blank update clears it, and a blank create omits it', () => {
  const form = resourceFormSchema(
    zSiteUpdateContactInput.pick({ email: true, firstName: true, tags: true }),
    { email: '', firstName: '', tags: [] as string[] },
  )
  const omitted = form.update.parse({ tags: [] })
  const cleared = form.update.parse({ email: null, firstName: '  ', tags: [] })
  const created = form.create.parse({ email: ' ada@example.com ', firstName: ' ', tags: [] })

  expect(omitted).toEqual({ tags: [] })
  expect(cleared).toEqual({ email: null, firstName: null, tags: [] })
  expect(created).toEqual({ email: 'ada@example.com', tags: [] })
})

test('a Rule keeps structured Conditions and Actions through hydration and unrelated edits', () => {
  const form = resourceFormSchema(zSiteCreateRuleInput, {
    name: '',
    triggerEvents: [] as string[],
    condition: emptyGroup,
    actions: [],
  })
  const loaded = form.toValues({
    name: 'Tag signups',
    triggerEvents: ['signed_up'],
    condition: {
      kind: 'group',
      combinator: 'and',
      rules: [{ kind: 'event', action: 'purchased', operator: 'atLeast', times: 2 }],
    },
    actions: [{ type: 'apply_tag', tag: 'new' }],
  })
  const payload = form.update.parse({ ...loaded, name: ' Renamed ' })

  expect(payload).toEqual({
    name: 'Renamed',
    triggerEvents: ['signed_up'],
    condition: {
      kind: 'group',
      combinator: 'and',
      rules: [{ kind: 'event', action: 'purchased', operator: 'atLeast', times: 2 }],
    },
    actions: [{ type: 'apply_tag', tag: 'new' }],
  })
})

test('an incomplete Condition stays in the editor and is refused at its missing value', () => {
  const form = resourceFormSchema(zSiteCreateRuleInput, {
    name: '',
    triggerEvents: [] as string[],
    condition: emptyGroup,
    actions: [],
  })
  const values = {
    ...form.blank,
    name: 'Buyers',
    triggerEvents: ['signed_up'],
    condition: {
      ...emptyGroup,
      rules: [{ kind: 'event', action: 'purchased', operator: 'atLeast' }],
    },
  }
  const submitted = form.create.safeParse(values)
  const issues = submitted.error?.issues.map((issue) => issue.path)

  expect(values.condition.rules).toEqual([
    { kind: 'event', action: 'purchased', operator: 'atLeast' },
  ])
  expect(issues).toEqual([['condition', 'rules', 0, 'times']])
})

test('a nullable marker cannot silently omit a nonnullable structured field', () => {
  const form = resourceFormSchema(zSiteCreateRuleInput, {
    name: '',
    triggerEvents: [] as string[],
    condition: emptyGroup,
    actions: [],
  })
  const submitted = form.update.safeParse({
    ...form.blank,
    name: 'Buyers',
    triggerEvents: ['signed_up'],
    condition: null,
  })
  const issues = submitted.error?.issues.map((issue) => issue.path)

  expect(issues).toEqual([['condition']])
})

test('invalid structured text is refused rather than normalized as a blank scalar', () => {
  const form = resourceFormSchema(zSiteUpdateContactInput.pick({ firstName: true, tags: true }), {
    firstName: '',
    tags: [] as string[],
  })
  const submitted = form.update.safeParse({ firstName: 'Ada', tags: '' })
  const issues = submitted.error?.issues.map((issue) => issue.path)

  expect(issues).toEqual([['tags']])
})

test('source-only messages omit the document and keep scalar clearing semantics', () => {
  const form = resourceFormSchema(zCreateTemplateInput, {
    name: '',
    subject: '',
    body: '',
    doc: null,
  })
  const loaded = form.toValues({ name: 'Welcome', subject: 'Hi', body: '<p>Hello</p>' })
  const payload = form.update.parse({ ...loaded, subject: '' })

  expect(loaded.doc).toBeNull()
  expect(payload).toEqual({ name: 'Welcome', subject: '', body: '<p>Hello</p>' })
})
