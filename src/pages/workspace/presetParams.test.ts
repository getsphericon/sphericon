import { expect, test } from 'vitest'

import type { SitePresetResource } from '../../generated/site/types.gen.ts'
import { formSchema, formUi, formValues, paramsToSave, savedSecrets } from './presetParams.ts'

const TEXTS = { keep: 'keep', rotates: 'rotates' }

const signed: SitePresetResource = {
  name: 'signed',
  description: 'A signed hook',
  paramsSchema: {
    type: 'object',
    required: ['secret', 'topic'],
    properties: {
      secret: { type: 'string', writeOnly: true },
      topic: { type: 'string' },
    },
  },
}

test('a Preset without properties has no params, so nothing is a secret', () => {
  const bare: SitePresetResource = {
    name: 'bare',
    description: '',
    paramsSchema: { type: 'object' },
  }

  expect(savedSecrets(bare, { secret: '********' })).toEqual(new Set())
  expect(formValues(bare, { secret: '********' })).toEqual({ secret: '********' })
  expect(formSchema(bare, new Set())).toEqual({ type: 'object' })
  expect(formUi(bare, new Set(), TEXTS)).toEqual({ 'ui:submitButtonOptions': { norender: true } })
})

test('a stored secret is neither filled in nor required, and a blank one is not saved', () => {
  const saved = savedSecrets(signed, { secret: '********', topic: 'acme' })

  expect(saved).toEqual(new Set(['secret']))
  expect(formValues(signed, { secret: '********', topic: 'acme' })).toEqual({ topic: 'acme' })
  expect(formSchema(signed, saved).required).toEqual(['topic'])
  expect(formUi(signed, saved, TEXTS).secret).toEqual({
    'ui:widget': 'password',
    'ui:placeholder': 'keep',
    'ui:help': 'rotates',
  })
  expect(paramsToSave(signed, { secret: '', topic: 'acme' })).toEqual({ topic: 'acme' })
  expect(paramsToSave(signed, { secret: 'new', topic: 'acme' })).toEqual({
    secret: 'new',
    topic: 'acme',
  })
})

test('a secret with no stored value stays required', () => {
  const saved = savedSecrets(signed, { secret: '' })

  expect(saved).toEqual(new Set())
  expect(formSchema(signed, saved).required).toEqual(['secret', 'topic'])
  expect(formUi(signed, saved, TEXTS).secret).toEqual({ 'ui:widget': 'password' })
})
