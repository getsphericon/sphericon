import { Badge, Card, Group, Loader, Stack, Text, Title } from '@mantine/core'
import { IconArrowLeft } from '@tabler/icons-react'
import { useQuery } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../components/ApiErrorAlert.tsx'
import { ButtonLink } from '../components/RouterLink.tsx'
import { operatorWorkspacesGetOptions } from '../generated/operator/@tanstack/react-query.gen.ts'
import type { OperatorSuspension } from '../generated/operator/types.gen.ts'
import { consoleHomeRoute, consoleWorkspaceRoute } from '../router.tsx'
import { formatDateTime } from '../utils/datetime.ts'
import { AuditLogCard } from './AuditLogCard.tsx'
import { DeliverabilityCard } from './deliverability.tsx'
import { SuspensionActions } from './SuspensionActions.tsx'

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <Group gap="xs" wrap="nowrap" align="baseline">
      <Text size="sm" c="dimmed" w={110} flex="none">
        {label}
      </Text>
      <Text size="sm">{children}</Text>
    </Group>
  )
}

function SuspensionCard({
  workspaceId,
  suspension,
}: {
  workspaceId: string
  suspension: OperatorSuspension | null | undefined
}) {
  const { t } = useTranslation()

  return (
    <Card withBorder>
      <Group justify="space-between" mb="sm">
        <Title order={4}>{t(($) => $.console.workspace.suspensionTitle)}</Title>
        {suspension ? (
          <Badge color="red" variant="light">
            {t(($) => $.console.workspaces.suspended)}
          </Badge>
        ) : (
          <Badge color="green" variant="light">
            {t(($) => $.console.workspaces.active)}
          </Badge>
        )}
      </Group>
      {suspension ? (
        <Stack gap="xs">
          <Field label={t(($) => $.console.workspace.suspendedAt)}>
            {formatDateTime(suspension.at)}
          </Field>
          <Field label={t(($) => $.console.workspace.suspendedBy)}>
            {suspension.actor.kind === 'operator' && suspension.actor.id
              ? t(($) => $.console.workspace.actorOperator, { id: suspension.actor.id })
              : t(($) => $.console.workspace.actors[suspension.actor.kind])}
          </Field>
          <Field label={t(($) => $.console.workspace.reason)}>
            {suspension.reason || t(($) => $.console.workspace.noReason)}
          </Field>
        </Stack>
      ) : (
        <Text size="sm" c="dimmed">
          {t(($) => $.console.workspace.notSuspended)}
        </Text>
      )}
      <Group mt="md">
        <SuspensionActions workspaceId={workspaceId} suspended={Boolean(suspension)} />
      </Group>
    </Card>
  )
}

// One Workspace: metadata and suspension state, as stacked cards so a later slice
// (rates, Audit log, suspend actions) adds a card without reshaping the page.
export function ConsoleWorkspacePage() {
  const { t } = useTranslation()
  const { workspaceId } = consoleWorkspaceRoute.useParams()
  const workspaceQuery = useQuery(operatorWorkspacesGetOptions({ path: { workspaceId } }))
  const workspace = workspaceQuery.data

  return (
    <Stack>
      <Group>
        <ButtonLink
          to={consoleHomeRoute.to}
          variant="subtle"
          leftSection={<IconArrowLeft size={16} />}
        >
          {t(($) => $.console.workspace.back)}
        </ButtonLink>
      </Group>
      {workspaceQuery.isLoading ? <Loader /> : null}
      {workspaceQuery.isError ? (
        <ApiErrorAlert
          error={workspaceQuery.error}
          title={t(($) => $.console.workspace.loadError)}
          fallback={t(($) => $.console.workspace.loadError)}
        />
      ) : null}
      {workspace ? (
        <>
          <Title order={2}>{workspace.name}</Title>
          <Card withBorder>
            <Title order={4} mb="sm">
              {t(($) => $.console.workspace.detailsTitle)}
            </Title>
            <Stack gap="xs">
              <Field label={t(($) => $.console.workspaces.slug)}>{workspace.slug}</Field>
              <Field label={t(($) => $.console.workspace.id)}>{workspace.id}</Field>
              <Field label={t(($) => $.console.workspaces.createdAt)}>
                {formatDateTime(workspace.createdAt)}
              </Field>
            </Stack>
          </Card>
          <SuspensionCard workspaceId={workspace.id} suspension={workspace.suspension} />
          <DeliverabilityCard deliverability={workspace.deliverability} />
          <AuditLogCard workspaceId={workspace.id} />
        </>
      ) : null}
    </Stack>
  )
}
