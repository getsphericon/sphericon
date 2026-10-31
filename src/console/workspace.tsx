import { Badge, Card, Group, Stack, Text, Title } from '@mantine/core'
import { IconArrowLeft } from '@tabler/icons-react'
import { useSuspenseQuery } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { QueryBoundary } from '../components/QueryBoundary.tsx'
import { ButtonLink } from '../components/RouterLink.tsx'
import { BlockSkeleton } from '../components/Skeletons.tsx'
import { staffWorkspacesGetOptions } from '../generated/staffee/@tanstack/react-query.gen.ts'
import type { StaffAbuse, StaffSuspension } from '../generated/staffee/types.gen.ts'
import { Route as consoleWorkspaceRoute } from '../routes/console/_authed/_enterprise/workspaces.$workspaceId.tsx'
import { Route as consoleWorkspacesRoute } from '../routes/console/_authed/_enterprise/workspaces.index.tsx'
import { formatDateTime } from '../utils/datetime.ts'
import { formatPercent } from '../utils/percent.ts'
import { AuditLogCard } from './AuditLogCard.tsx'
import { DeliverabilityCard } from './deliverability.tsx'
import { HoldsCard } from './HoldsCard.tsx'
import { SendCapCard } from './SendCapCard.tsx'
import { ExemptionAction, SuspensionActions } from './SuspensionActions.tsx'

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
  suspension: StaffSuspension | null | undefined
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
            {suspension.actor.kind === 'staff' && suspension.actor.id
              ? t(($) => $.console.workspace.actorStaff, { id: suspension.actor.id })
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

// The Workspace's standing with the abuse detector (ADR 0026): whether it is exempt, the
// warning it is in right now (computed live), and when the owners were last told.
function AbuseCard({ workspaceId, abuse }: { workspaceId: string; abuse: StaffAbuse }) {
  const { t } = useTranslation()
  const never = t(($) => $.console.workspace.abuse.never)
  const warning = abuse.warning

  return (
    <Card withBorder>
      <Group justify="space-between" mb="sm">
        <Title order={4}>{t(($) => $.console.workspace.abuse.title)}</Title>
        {abuse.exempt ? (
          <Badge color="grape" variant="light">
            {t(($) => $.console.workspace.abuse.exempt)}
          </Badge>
        ) : (
          <Badge color="gray" variant="light">
            {t(($) => $.console.workspace.abuse.policed)}
          </Badge>
        )}
      </Group>
      <Stack gap="xs">
        <Field label={t(($) => $.console.workspace.abuse.warning)}>
          {warning
            ? t(($) => $.console.workspace.abuse.warningDetail, {
                metric: t(($) => $.console.workspace.abuse.metrics[warning.metric]),
                rate: formatPercent(warning.rate),
                domain: warning.domain,
              })
            : t(($) => $.console.workspace.abuse.noWarning)}
        </Field>
        <Field label={t(($) => $.console.workspace.abuse.clearedAt)}>
          {abuse.clearedAt ? formatDateTime(abuse.clearedAt) : never}
        </Field>
        <Field label={t(($) => $.console.workspace.abuse.warningNotifiedAt)}>
          {abuse.warningNotifiedAt ? formatDateTime(abuse.warningNotifiedAt) : never}
        </Field>
        <Field label={t(($) => $.console.workspace.abuse.freezeNotifiedAt)}>
          {abuse.freezeNotifiedAt ? formatDateTime(abuse.freezeNotifiedAt) : never}
        </Field>
      </Stack>
      <Group mt="md">
        <ExemptionAction workspaceId={workspaceId} exempt={abuse.exempt} />
      </Group>
    </Card>
  )
}

// The cards of the Workspace; the page's boundary shows the load and its failure.
function WorkspaceCards({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation()
  const workspace = useSuspenseQuery(staffWorkspacesGetOptions({ path: { workspaceId } })).data

  return (
    <>
      <Title order={2}>{workspace.name}</Title>
      <Card withBorder>
        <Title order={4} mb="sm">
          {t(($) => $.console.workspace.detailsTitle)}
        </Title>
        <Stack gap="xs">
          <Field label={t(($) => $.console.workspace.id)}>{workspace.id}</Field>
          <Field label={t(($) => $.console.workspaces.createdAt)}>
            {formatDateTime(workspace.createdAt)}
          </Field>
        </Stack>
      </Card>
      <SuspensionCard workspaceId={workspace.id} suspension={workspace.suspension} />
      <AbuseCard workspaceId={workspace.id} abuse={workspace.abuse} />
      {workspace.sendCaps && (
        <SendCapCard
          workspaceId={workspace.id}
          sendCap={workspace.sendCap}
          sentLast24h={workspace.sentLast24h}
        />
      )}
      <HoldsCard holds={workspace.holds} />
      <DeliverabilityCard deliverability={workspace.deliverability} />
      <AuditLogCard workspaceId={workspace.id} />
    </>
  )
}

// One Workspace: metadata and suspension state, as stacked cards so a later slice
// (rates, Audit log, suspend actions) adds a card without reshaping the page.
export function ConsoleWorkspacePage() {
  const { t } = useTranslation()
  const { workspaceId } = consoleWorkspaceRoute.useParams()

  return (
    <Stack>
      <Group>
        <ButtonLink
          to={consoleWorkspacesRoute.to}
          variant="subtle"
          leftSection={<IconArrowLeft size={16} />}
        >
          {t(($) => $.console.workspace.back)}
        </ButtonLink>
      </Group>
      <QueryBoundary
        fallback={<BlockSkeleton />}
        errorTitle={t(($) => $.console.workspace.loadError)}
      >
        <WorkspaceCards workspaceId={workspaceId} />
      </QueryBoundary>
    </Stack>
  )
}
