import { expect, test } from 'vitest'

import type { ContactImportMapping } from '../../generated/site/types.gen.ts'
import { zConfirmationProvenance, zSuppressionReason } from '../../generated/site/zod.gen.ts'
import { emptyFact } from './importFacts.ts'
import {
  columnsMissingDateFormat,
  mappingInputOf,
  targets,
  valuesOf,
  writesDate,
} from './importMapping.ts'

const fields = [
  { key: 'plan', type: 'string' as const },
  { key: 'renews_at', type: 'datetime' as const },
]

const proposal: ContactImportMapping = {
  columns: [
    { kind: 'key', key: 'email' },
    { kind: 'attribute', attribute: 'birthday', dateFormat: null },
    { kind: 'customField', key: 'renews_at', dateFormat: 'dd/mm/yyyy' },
    { kind: 'tags' },
    { kind: 'ignore' },
  ],
  decimalSeparator: 'comma',
  defaultCountry: null,
  tag: 'Imported 2026-11-27',
  onlyAddNew: false,
  asNewSignUps: true,
}

test('a mapping read back is the mapping a request saves', () => {
  const input = mappingInputOf(valuesOf(proposal), fields)

  expect(input).toEqual({ ...proposal, columns: proposal.columns })
})

test('a date column without a format blocks the run until one is chosen', () => {
  const values = valuesOf(proposal)

  expect(columnsMissingDateFormat(values, fields)).toEqual([1])
})

test('only the birthday and a datetime field write a date', () => {
  expect(writesDate('attribute:birthday', fields)).toBe(true)
  expect(writesDate('customField:renews_at', fields)).toBe(true)
  expect(writesDate('customField:plan', fields)).toBe(false)
  expect(writesDate('attribute:firstName', fields)).toBe(false)
})

test('a column moved off a date drops its format, and a blank Tag is none', () => {
  const values = valuesOf(proposal)
  values.columns[2] = { target: 'customField:plan', dateFormat: 'dd/mm/yyyy' }
  values.tag = '  '

  const input = mappingInputOf(values, fields)

  expect(input.columns[2]).toEqual({ kind: 'customField', key: 'plan', dateFormat: null })
  expect(input.tag).toBeNull()
})

test('the targets are every key, every attribute and each Custom field', () => {
  const all = targets(fields)

  expect(all.keys).toContainEqual({ value: 'key:subjectId', key: 'subjectId' })
  expect(all.attributes).toContainEqual({ value: 'attribute:birthday', attribute: 'birthday' })
  expect(all.customFields.map((field) => field.value)).toEqual([
    'customField:plan',
    'customField:renews_at',
  ])
})

test('a mapping of refusals, consent, tracking and their times reads back as it is saved', () => {
  const facts: ContactImportMapping = {
    ...proposal,
    columns: [
      { kind: 'key', key: 'email' },
      {
        kind: 'refusal',
        channel: 'email',
        values: [
          { value: 'unsubscribed', refusal: 'unsubscribeEverything' },
          { value: 'bounced', refusal: 'suppression' },
        ],
      },
      { kind: 'time', of: 1, dateFormat: 'rfc3339' },
      {
        kind: 'consent',
        channel: 'email',
        values: ['yes'],
        evidence: {
          wording: 'Send me news',
          wordingColumn: null,
          source: null,
          sourceColumn: 4,
          brand: 'Acme',
          brandColumn: null,
        },
      },
      { kind: 'engagementTracking', channel: 'sms', granted: ['y'], denied: ['n'] },
    ],
  }

  const input = mappingInputOf(valuesOf(facts), fields)

  expect(input.columns).toEqual(facts.columns)
  expect(columnsMissingDateFormat(valuesOf(facts), fields)).toEqual([])
})

test('a time column needs its format before the run', () => {
  const values = valuesOf({
    ...proposal,
    columns: [{ kind: 'time', of: 0, dateFormat: 'rfc3339' }],
  })
  const cleared = { ...values, columns: values.columns.map((c) => ({ ...c, dateFormat: null })) }

  expect(writesDate('time', fields)).toBe(true)
  expect(columnsMissingDateFormat(cleared, fields)).toEqual([0])
})

// The proposal for a CSV export of Contacts (ADR 0138) holds every kind of column, its consent's
// wording and brand null (an export holds neither); a Member who changes nothing sends it as it was
// proposed, and the save refuses the blank evidence.
const exportProposal: ContactImportMapping = {
  columns: [
    { kind: 'key', key: 'email' },
    { kind: 'customField', key: 'plan', dateFormat: null },
    { kind: 'newCustomField', key: 'score', label: 'score', type: 'number', dateFormat: null },
    {
      kind: 'newCustomField',
      key: 'seen_at',
      label: 'seen_at',
      type: 'datetime',
      dateFormat: 'rfc3339',
    },
    {
      kind: 'refusal',
      channel: 'email',
      values: [
        { value: 'everything', refusal: 'unsubscribeEverything' },
        { value: 'broadcasts', refusal: 'unsubscribeBroadcasts' },
      ],
    },
    { kind: 'time', of: 4, dateFormat: 'rfc3339' },
    {
      kind: 'refusal',
      channel: 'email',
      values: zSuppressionReason.options.map((value) => ({
        value,
        refusal: 'suppression' as const,
      })),
    },
    {
      kind: 'consent',
      channel: 'email',
      values: [...zConfirmationProvenance.options],
      evidence: {
        wording: null,
        wordingColumn: null,
        source: null,
        sourceColumn: 7,
        brand: null,
        brandColumn: null,
      },
    },
    { kind: 'time', of: 7, dateFormat: 'rfc3339' },
    { kind: 'tags' },
    { kind: 'ignore' },
  ],
  decimalSeparator: 'point',
  defaultCountry: null,
  tag: 'Imported 2026-11-27',
  onlyAddNew: false,
  asNewSignUps: false,
}

test("an export's proposal is saved as it was proposed", () => {
  const input = mappingInputOf(valuesOf(exportProposal), fields)

  expect(input).toEqual(exportProposal)
})

// A Telegram refusal column turned into a consent or tracking column, which Telegram has none of, is
// never sent as one of email: the Member chooses its Channel first.
test('a consent or tracking column on Telegram is not sent as one on email', () => {
  for (const kind of ['consent', 'engagementTracking'] as const) {
    const values = valuesOf(proposal)
    values.columns[4] = {
      target: kind,
      dateFormat: null,
      fact: { ...emptyFact(), channel: 'telegram', values: ['yes'] },
    }

    expect(() => mappingInputOf(values, fields)).toThrow(
      'a consent or tracking column is not on telegram',
    )
  }
})
