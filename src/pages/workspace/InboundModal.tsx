import {
  Alert,
  Button,
  Code,
  CopyButton,
  Group,
  JsonInput,
  Modal,
  Select,
  Spoiler,
  Stack,
  Text,
} from '@mantine/core'
import { modals } from '@mantine/modals'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  siteIntegrationsListQueryKey,
  siteIntegrationsRegenerateKeyMutation,
  siteIntegrationsUpdateMutation,
  sitePresetsListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteIntegrationResource, SitePresetResource } from '../../generated/site/types.gen.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'

interface InboundModalProps {
  workspaceId: string
  integration: SiteIntegrationResource | null
  onClose: () => void
}

// The inbound side of an Integration (ADR 0037): its hook URL, the Preset its provider
// callbacks are read with and that Preset's params. Agents are the primary interface; this
// is for support, so the params are a JSON editor, not a form generated from the schema.
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

// A skeleton of the params: every property of the schema, empty. A secret stays empty:
// an empty secret means unchanged.
function skeleton(preset: SitePresetResource | undefined): string {
  const properties = preset?.paramsSchema.properties
  const keys = typeof properties === 'object' && properties !== null ? Object.keys(properties) : []
  return JSON.stringify(Object.fromEntries(keys.map((key) => [key, ''])), null, 2)
}

function parseParams(text: string): Record<string, unknown> | null {
  try {
    const parsed: unknown = JSON.parse(text)
    if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
      return null
    }
    const values: Record<string, unknown> = {}
    for (const [key, value] of Object.entries(parsed)) {
      values[key] = value
    }
    return values
  } catch {
    return null
  }
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
  const [params, setParams] = useState(JSON.stringify(integration.params, null, 2))
  const selected = presets.find((p) => p.name === preset)
  const invalid = preset !== null && parseParams(params) === null

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
    setParams(
      value === integration.preset
        ? JSON.stringify(integration.params, null, 2)
        : skeleton(presets.find((p) => p.name === value)),
    )
  }

  const onSave = () => {
    const values = preset === null ? null : parseParams(params)
    saveMutation.mutate({
      path: { workspaceId, id: integration.id },
      body: values ? { preset, params: values } : { preset },
    })
  }

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

      {preset === null ? null : (
        <>
          <JsonInput
            label={t(($) => $.settings.integrations.inbound.params)}
            description={t(($) => $.settings.integrations.inbound.paramsHint)}
            validationError={t(($) => $.settings.integrations.inbound.paramsInvalid)}
            formatOnBlur
            autosize
            minRows={4}
            value={params}
            onChange={setParams}
          />
          {selected ? (
            <Spoiler
              maxHeight={0}
              showLabel={t(($) => $.settings.integrations.inbound.showSchema)}
              hideLabel={t(($) => $.settings.integrations.inbound.hideSchema)}
            >
              <Code block>{JSON.stringify(selected.paramsSchema, null, 2)}</Code>
            </Spoiler>
          ) : null}
        </>
      )}

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

      <Group justify="flex-end">
        <Button disabled={invalid} loading={saveMutation.isPending} onClick={onSave}>
          {t(($) => $.settings.integrations.inbound.save)}
        </Button>
      </Group>
    </Stack>
  )
}
