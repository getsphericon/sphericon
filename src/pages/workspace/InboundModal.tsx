import { Alert, Button, Code, CopyButton, Group, Modal, Stack, Text } from '@mantine/core'
import { modals } from '@mantine/modals'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { BlockSkeleton } from '../../components/Skeletons.tsx'
import { siteIntegrationsGetOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteIntegrationResource } from '../../generated/site/types.gen.ts'
import type { ResourceEditor } from '../../resources/defineResource.tsx'
import { hasErrorStatus } from '../../utils/apiErrors.ts'
import { InboundEditForm } from './inboundEditor.ts'
import type { inboundOperations } from './inboundEditor.ts'
import { useIntegrationGone } from './integrationGone.ts'

interface InboundModalProps {
  workspaceId: string
  integration: SiteIntegrationResource | null
  onClose: () => void
}

type InboundOperation = keyof typeof inboundOperations

// The inbound side of an Integration (ADR 0037): its hook URL, the Preset its provider
// callbacks are read with and that Preset's params, in a form generated from the Preset's schema
// (ADR 0066). It stays open across its writes, which one editor session serializes.
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
        <QueryBoundary
          fallback={<BlockSkeleton />}
          errorTitle={t(($) => $.settings.integrations.loadError)}
        >
          <LoadedInbound workspaceId={workspaceId} id={integration.id} onClose={onClose} />
        </QueryBoundary>
      ) : null}
    </Modal>
  )
}

function LoadedInbound({
  workspaceId,
  id,
  onClose,
}: {
  workspaceId: string
  id: string
  onClose: () => void
}) {
  const { t } = useTranslation()
  const { data } = useSuspenseQuery(siteIntegrationsGetOptions({ path: { workspaceId, id } }))
  const gone = useIntegrationGone(workspaceId, onClose)
  const errorTitle = t(($) => $.settings.integrations.inbound.saveError)
  return (
    <InboundEditForm
      key={id}
      workspaceId={workspaceId}
      id={id}
      loaded={data}
      titled={false}
      // An Integration deleted while the window was open closes it and reloads the list.
      onFailed={(error) => {
        if (!hasErrorStatus(error, [404])) {
          return false
        }
        void gone(errorTitle)(error)
        return true
      }}
    >
      {(stored, editor) => <InboundHook stored={stored} editor={editor} />}
    </InboundEditForm>
  )
}

// The hook URL with its copy and its regeneration, and the old secrets of a rotation to retire: the
// stored Integration's, as the session shows it.
function InboundHook({
  stored,
  editor,
}: {
  stored: SiteIntegrationResource
  editor: ResourceEditor<InboundOperation>
}) {
  const { t } = useTranslation()
  const telegram = stored.provider === 'telegram'

  const onRegenerate = () => {
    modals.openConfirmModal({
      title: t(($) => $.settings.integrations.inbound.regenerateConfirmTitle),
      children: (
        <Text>
          {telegram
            ? t(($) => $.settings.integrations.inbound.telegramRegenerateConfirm)
            : t(($) => $.settings.integrations.inbound.regenerateConfirm)}
        </Text>
      ),
      labels: {
        cancel: t(($) => $.actions.cancel),
        confirm: t(($) => $.settings.integrations.inbound.regenerate),
      },
      confirmProps: { color: 'red' },
      onConfirm: () => {
        void editor.run('REGENERATE')
      },
    })
  }

  return (
    <Stack>
      <Stack gap={4}>
        <Text size="sm" fw={500}>
          {t(($) => $.settings.integrations.inbound.hookUrl)}
        </Text>
        <Code block>{stored.hookUrl}</Code>
        <Group>
          <CopyButton value={stored.hookUrl}>
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
            loading={editor.operation === 'REGENERATE'}
            disabled={editor.busy}
            onClick={onRegenerate}
          >
            {t(($) => $.settings.integrations.inbound.regenerate)}
          </Button>
        </Group>
      </Stack>
      {stored.secretsRetiring.length > 0 ? (
        <Alert
          color="yellow"
          variant="light"
          title={t(($) => $.settings.integrations.inbound.rotating)}
        >
          <Stack gap="xs" align="flex-start">
            <Text size="sm">
              {t(($) => $.settings.integrations.inbound.rotatingDescription, {
                names: stored.secretsRetiring.join(', '),
              })}
            </Text>
            <Button
              size="compact-sm"
              variant="light"
              loading={editor.operation === 'RETIRE'}
              disabled={editor.busy}
              onClick={() => {
                void editor.run('RETIRE')
              }}
            >
              {t(($) => $.settings.integrations.inbound.retire)}
            </Button>
          </Stack>
        </Alert>
      ) : null}
    </Stack>
  )
}
