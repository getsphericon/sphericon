import { Alert, Button, Code, CopyButton, Group, Modal, Select, Stack, Text } from '@mantine/core'
import { modals } from '@mantine/modals'
import { Form } from '@rjsf/mantine'
import { customizeValidator } from '@rjsf/validator-ajv8'
import { useQuery } from '@tanstack/react-query'
import ajvFormats from 'ajv-formats'
import { Ajv2020 } from 'ajv/dist/2020.js'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  siteIntegrationsListQueryKey,
  siteIntegrationsRegenerateKeyMutation,
  siteIntegrationsUpdateMutation,
  sitePresetsListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteIntegrationResource } from '../../generated/site/types.gen.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import {
  formSchema,
  formUi,
  formValues,
  paramsToSave,
  savedSecrets,
  serverErrors,
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

interface InboundModalProps {
  workspaceId: string
  integration: SiteIntegrationResource | null
  onClose: () => void
}

// The inbound side of an Integration (ADR 0037): its hook URL, the Preset its provider
// callbacks are read with and that Preset's params, in a form generated from the Preset's schema
// (ADR 0066).
export function InboundModal({ workspaceId, integration, onClose }: InboundModalProps) {
  const { t } = useTranslation()
  return (
    <Modal
      opened={integration !== null}
      onClose={onClose}
      size="lg"
      title={t(($) => $.settings.integrations.inbound.title, { name: integration?.name ?? '' })}
    >
      {integration ? (
        <InboundForm key={integration.id} workspaceId={workspaceId} integration={integration} />
      ) : null}
    </Modal>
  )
}

function InboundForm({
  workspaceId,
  integration,
}: {
  workspaceId: string
  integration: SiteIntegrationResource
}) {
  const { t } = useTranslation()
  const presetsQuery = useQuery(sitePresetsListOptions())
  const presets = presetsQuery.data ?? []

  const [preset, setPreset] = useState<string | null>(integration.preset)
  // The stored params belong to the stored Preset: a change of Preset starts from none.
  const [values, setValues] = useState<PresetParams>(integration.params)
  const selected = presets.find((p) => p.name === preset)
  const own = preset === integration.preset
  // Only the stored Preset's secrets have a stored value to keep; no secret carries over.
  const saved = selected && own ? savedSecrets(selected, integration.params) : new Set<string>()

  const invalidate = [siteIntegrationsListQueryKey({ path: { workspaceId } })]
  const saveMutation = useResourceMutation({
    mutation: siteIntegrationsUpdateMutation(),
    invalidate,
    successMessage: t(($) => $.settings.integrations.inbound.saved),
    errorTitle: t(($) => $.settings.integrations.inbound.saveError),
  })
  const retireMutation = useResourceMutation({
    mutation: siteIntegrationsUpdateMutation(),
    invalidate,
    successMessage: t(($) => $.settings.integrations.inbound.retired),
    errorTitle: t(($) => $.settings.integrations.inbound.saveError),
  })
  const regenerateMutation = useResourceMutation({
    mutation: siteIntegrationsRegenerateKeyMutation(),
    invalidate,
    successMessage: t(($) => $.settings.integrations.inbound.regenerated),
    errorTitle: t(($) => $.settings.integrations.inbound.regenerateError),
  })

  const onPresetChange = (value: string | null) => {
    setPreset(value)
    setValues(value === integration.preset ? integration.params : {})
    saveMutation.reset()
  }

  const save = (params?: PresetParams) =>
    saveMutation.mutate({
      path: { workspaceId, id: integration.id },
      body: params ? { preset, params } : { preset },
    })

  const onRegenerate = () => {
    modals.openConfirmModal({
      title: t(($) => $.settings.integrations.inbound.regenerateConfirmTitle),
      children: <Text>{t(($) => $.settings.integrations.inbound.regenerateConfirm)}</Text>,
      labels: {
        cancel: t(($) => $.actions.cancel),
        confirm: t(($) => $.settings.integrations.inbound.regenerate),
      },
      confirmProps: { color: 'red' },
      onConfirm: () => regenerateMutation.mutate({ path: { workspaceId, id: integration.id } }),
    })
  }

  return (
    <Stack>
      <Text c="dimmed" size="sm">
        {t(($) => $.settings.integrations.inbound.description)}
      </Text>

      <Stack gap={4}>
        <Text size="sm" fw={500}>
          {t(($) => $.settings.integrations.inbound.hookUrl)}
        </Text>
        <Code block>{integration.hookUrl}</Code>
        <Group>
          <CopyButton value={integration.hookUrl}>
            {({ copied, copy }) => (
              <Button size="compact-sm" variant="light" onClick={copy}>
                {copied ? t(($) => $.settings.copied) : t(($) => $.settings.copy)}
              </Button>
            )}
          </CopyButton>
          <Button
            size="compact-sm"
            color="red"
            variant="light"
            loading={regenerateMutation.isPending}
            onClick={onRegenerate}
          >
            {t(($) => $.settings.integrations.inbound.regenerate)}
          </Button>
        </Group>
      </Stack>

      <Select
        label={t(($) => $.settings.integrations.inbound.preset)}
        placeholder={t(($) => $.settings.integrations.inbound.noPreset)}
        data={presets.map((p) => ({ value: p.name, label: p.name }))}
        description={selected?.description}
        clearable
        value={preset}
        onChange={onPresetChange}
      />

      {selected ? (
        <Form
          key={selected.name}
          schema={formSchema(selected, saved)}
          uiSchema={formUi(selected, saved, {
            keep: t(($) => $.settings.integrations.inbound.secretKeep),
            rotates: t(($) => $.settings.integrations.inbound.secretRotates),
          })}
          formData={formValues(selected, values)}
          validator={validator}
          showErrorList={false}
          extraErrors={serverErrors(saveMutation.error)}
          onChange={({ formData }) => setValues(formData ?? {})}
          onSubmit={({ formData }) => save(paramsToSave(selected, formData ?? {}))}
        >
          <Group justify="flex-end" mt="md">
            <Button type="submit" loading={saveMutation.isPending}>
              {t(($) => $.settings.integrations.inbound.save)}
            </Button>
          </Group>
        </Form>
      ) : null}

      {integration.secretsRetiring.length > 0 ? (
        <Alert
          color="yellow"
          variant="light"
          title={t(($) => $.settings.integrations.inbound.rotating)}
        >
          <Stack gap="xs" align="flex-start">
            <Text size="sm">
              {t(($) => $.settings.integrations.inbound.rotatingDescription, {
                names: integration.secretsRetiring.join(', '),
              })}
            </Text>
            <Button
              size="compact-sm"
              variant="light"
              loading={retireMutation.isPending}
              onClick={() =>
                retireMutation.mutate({
                  path: { workspaceId, id: integration.id },
                  body: { retireSecrets: true },
                })
              }
            >
              {t(($) => $.settings.integrations.inbound.retire)}
            </Button>
          </Stack>
        </Alert>
      ) : null}

      {preset === null ? (
        <Group justify="flex-end">
          <Button loading={saveMutation.isPending} onClick={() => save()}>
            {t(($) => $.settings.integrations.inbound.save)}
          </Button>
        </Group>
      ) : null}
    </Stack>
  )
}
