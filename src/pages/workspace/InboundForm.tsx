import { Button, Group, Select, Stack, Text } from '@mantine/core'
import { Form } from '@rjsf/mantine'
import { customizeValidator } from '@rjsf/validator-ajv8'
import { useQuery } from '@tanstack/react-query'
import ajvFormats from 'ajv-formats'
import { Ajv2020 } from 'ajv/dist/2020.js'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { sitePresetsListOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteIntegrationResource, SitePresetResource } from '../../generated/site/types.gen.ts'
import type { ResourceFormProps } from '../../resources/defineResource.tsx'
import {
  formSchema,
  formUi,
  formValues,
  paramsToSave,
  savedSecrets,
  serverErrors,
  unmasked,
} from './presetParams.ts'
import type { PresetParams } from './presetParams.ts'

// A Preset's params are full JSON Schema 2020-12 (a custom Preset may use any of it), so the form
// validates by an Ajv of that draft, set up as rjsf sets up its own, `format`s included
// (ajv-formats, called by `.default`: under nodenext its default import is the CommonJS module).
// The server stays the authority: its refusal lands on the fields too.
const validator = customizeValidator<PresetParams>({
  extenderFn: () =>
    ajvFormats.default(
      new Ajv2020({ allErrors: true, multipleOfPrecision: 8, strict: false, verbose: true }),
    ),
})

// What the inbound side of an Integration holds: its Preset and that Preset's params. A param
// left as read keeps its stored value, a masked secret included (the server keeps it).
export interface InboundValues {
  preset: string | null
  params: PresetParams
}

// What a save sends: the Preset, and its params when it has one.
export interface InboundPayload {
  preset: string | null
  params?: PresetParams
}

// The Preset's params form. It is mounted per Preset and keeps what it shows itself, a secret being
// typed included, starting from the params without their secrets (never filled in); the session
// holds what a save sends, the blank secrets left out.
function PresetParamsForm({
  preset,
  stored,
  params,
  failure,
  isPending,
  onChange,
  onSubmit,
}: {
  preset: SitePresetResource
  stored: SiteIntegrationResource
  params: PresetParams
  failure: unknown
  isPending: boolean
  onChange: (params: PresetParams) => void
  onSubmit: () => void
}) {
  const { t } = useTranslation()
  // Only the stored Preset's secrets have a stored value to keep; no secret carries over.
  const saved =
    preset.name === stored.preset ? savedSecrets(preset, stored.params) : new Set<string>()
  const [initial] = useState(() => formValues(preset, params))
  return (
    <Form
      schema={formSchema(preset, saved)}
      uiSchema={formUi(preset, saved, {
        keep: t(($) => $.settings.integrations.inbound.secretKeep),
        rotates: t(($) => $.settings.integrations.inbound.secretRotates),
      })}
      formData={initial}
      validator={validator}
      showErrorList={false}
      extraErrors={serverErrors(failure)}
      onChange={({ formData }) => onChange(paramsToSave(preset, formData ?? {}))}
      onSubmit={onSubmit}
    >
      <Group justify="flex-end" mt="md">
        <Button type="submit" loading={isPending}>
          {t(($) => $.settings.integrations.inbound.save)}
        </Button>
      </Group>
    </Form>
  )
}

// The inbound side's Preset and params (ADR 0037, ADR 0066), in a form generated from the Preset's
// schema. A Telegram bot's and a Twilio account's Preset is fixed; a Twilio Auth Token is a param
// to rotate here.
export function InboundForm({
  form,
  isPending,
  onSubmit,
  stored,
  failure,
}: ResourceFormProps<InboundPayload, InboundValues, SiteIntegrationResource>) {
  const { t } = useTranslation()
  const telegram = stored?.provider === 'telegram'
  const fixed = telegram || stored?.provider === 'twilio'
  const presetsQuery = useQuery({ ...sitePresetsListOptions(), enabled: !telegram })
  if (!stored) {
    return null
  }
  const presets = presetsQuery.data ?? []
  const { preset, params } = form.values
  const selected = presets.find((p) => p.name === preset)
  const submit = () => onSubmit(preset === null ? { preset } : { preset, params })

  return (
    <Stack>
      <Text c="dimmed" size="sm">
        {telegram
          ? t(($) => $.settings.integrations.inbound.telegramDescription)
          : t(($) => $.settings.integrations.inbound.description)}
      </Text>
      {fixed ? null : (
        <Select
          label={t(($) => $.settings.integrations.inbound.preset)}
          placeholder={t(($) => $.settings.integrations.inbound.noPreset)}
          data={presets.map((p) => ({ value: p.name, label: p.name }))}
          description={selected?.description}
          clearable
          value={preset}
          // The stored params belong to the stored Preset: a change of Preset starts from none.
          onChange={(value) =>
            form.setValues({
              preset: value,
              params: value === stored.preset ? unmasked(stored.params) : {},
            })
          }
        />
      )}
      {selected ? (
        <PresetParamsForm
          key={selected.name}
          preset={selected}
          stored={stored}
          params={params}
          failure={failure}
          isPending={isPending}
          onChange={(next) => form.setFieldValue('params', next)}
          onSubmit={submit}
        />
      ) : null}
      {preset === null && !fixed ? (
        <Group justify="flex-end">
          <Button loading={isPending} onClick={submit}>
            {t(($) => $.settings.integrations.inbound.save)}
          </Button>
        </Group>
      ) : null}
    </Stack>
  )
}
