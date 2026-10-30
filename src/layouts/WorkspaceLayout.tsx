import { Alert, Anchor, Group, Select, Text } from '@mantine/core'
import { IconPlayerPause, IconPlus } from '@tabler/icons-react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useMatchRoute, useNavigate } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { AppNavbar } from '../components/AppNavbar.tsx'
import { InstanceNotice } from '../components/InstanceNotice.tsx'
import { LicenseBanner } from '../components/LicenseBanner.tsx'
import { SecondFactorBlocked, SecondFactorGraceBanner } from '../components/SecondFactorNotice.tsx'
import { SettingsNavbar } from '../components/SettingsNavbar.tsx'
import { UpdateBanner } from '../components/UpdateBanner.tsx'
import { UserMenu } from '../components/UserMenu.tsx'
import { siteWorkspacesListOptions } from '../generated/site/@tanstack/react-query.gen.ts'
import type { SiteWorkspaceHold, SiteWorkspaceResource } from '../generated/site/types.gen.ts'
import { Route as overviewRoute } from '../routes/workspaces/$workspaceId/index.tsx'
import { Route as workspaceRoute } from '../routes/workspaces/$workspaceId/route.tsx'
import { Route as settingsRoute } from '../routes/workspaces/$workspaceId/settings/route.tsx'
import { Route as newWorkspaceRoute } from '../routes/workspaces/new.tsx'
import { isSecondFactorRequiredError } from '../utils/apiErrors.ts'
import { can } from '../utils/permissions.ts'
import { secondFactorStanding } from '../utils/secondFactorStanding.ts'
import { DashboardShell } from './DashboardShell.tsx'

// The last option of the switcher is not a Workspace: choosing it opens the create screen.
const CREATE_OPTION = '+new'

// WorkspaceSwitcher changes the active workspace by navigating to its
// /w/{workspaceId} route. Each option shows the Workspace's id under its name, to tell
// same-named ones apart and to match the id in the URL; the last option creates a Workspace.
function WorkspaceSwitcher({
  workspaceId,
  workspaces,
}: {
  workspaceId: string
  workspaces: SiteWorkspaceResource[]
}) {
  const navigate = useNavigate()
  const { t } = useTranslation()
  const createLabel = t(($) => $.createWorkspace.switcherLabel)
  const data = [
    ...workspaces.map((w) => ({ value: w.id, label: w.name })),
    { value: CREATE_OPTION, label: createLabel },
  ]

  return (
    <Select
      data={data}
      value={workspaceId}
      allowDeselect={false}
      // The closed select is narrow on a phone: the list sizes to its options, or they wrap and clip.
      comboboxProps={{ width: 'max-content', position: 'bottom-end' }}
      renderOption={({ option }) =>
        option.value === CREATE_OPTION ? (
          <Group gap="xs" wrap="nowrap">
            <IconPlus size={16} />
            <Text size="sm">{option.label}</Text>
          </Group>
        ) : (
          <div>
            <Text size="sm">{option.label}</Text>
            <Text size="xs" c="dimmed">
              {option.value}
            </Text>
          </div>
        )
      }
      onChange={(value) => {
        if (value === CREATE_OPTION) {
          void navigate({ to: newWorkspaceRoute.to })
        } else if (value && value !== workspaceId) {
          void navigate({ to: overviewRoute.to, params: { workspaceId: value } })
        }
      }}
      flex={1}
      miw={0}
      maw={220}
    />
  )
}

// SuspensionBanner tells the owner that outbound sending is frozen and why (ADR 0007).
// Login, reads and tracking keep working, so it is a notice, not a lockout.
function SuspensionBanner({
  reason,
  supportEmail,
}: {
  reason: string | null | undefined
  supportEmail: string | null | undefined
}) {
  const { t } = useTranslation()
  return (
    <Alert color="red" title={t(($) => $.workspaceSuspension.title)} mb="md">
      {t(($) => $.workspaceSuspension.description)}
      {reason ? ` ${t(($) => $.workspaceSuspension.reason, { reason })}` : ''}{' '}
      {supportEmail
        ? t(($) => $.workspaceSuspension.appealSupport, { email: supportEmail })
        : t(($) => $.workspaceSuspension.appealAdmin)}
    </Alert>
  )
}

// HoldAction points a member who holds holds:resolve to where the setter of a hold says it can
// be resolved, and tells anyone else to ask an Owner or Admin; a hold with no action URL renders
// nothing.
function HoldAction({
  canResolve,
  actionUrl,
}: {
  canResolve: boolean
  actionUrl: string | null | undefined
}) {
  const { t } = useTranslation()
  if (!actionUrl) {
    return null
  }
  return canResolve ? (
    <Anchor size="sm" display="block" href={actionUrl}>
      {t(($) => $.workspaceHold.actionLink)}
    </Anchor>
  ) : (
    <Text size="sm">{t(($) => $.workspaceHold.actionContact)}</Text>
  )
}

// HoldBanner tells every member that outbound sending is held (ADR 0009) and shows each
// setter's message as given, with the link to resolve it when the setter gave one. It is
// deliberately not the suspension banner: another color and icon, another title.
function HoldBanner({
  holds,
  workspace,
}: {
  holds: SiteWorkspaceHold[]
  workspace: SiteWorkspaceResource
}) {
  const { t } = useTranslation()
  const canResolve = can(workspace, 'holds:resolve')
  return (
    <Alert
      color="yellow"
      variant="outline"
      icon={<IconPlayerPause />}
      title={t(($) => $.workspaceHold.title)}
      mb="md"
    >
      <Text size="sm">{t(($) => $.workspaceHold.description)}</Text>
      {holds.map((hold) => (
        <div key={hold.key}>
          <Text size="sm" fw={500} mt="xs">
            {hold.message}
          </Text>
          <HoldAction canResolve={canResolve} actionUrl={hold.actionUrl} />
        </div>
      ))}
    </Alert>
  )
}

// useSecondFactorRefused reports whether any query of the Workspace was refused
// with 403 second_factor_required since the Workspace list last loaded (listedAt):
// the server's word that the grace is over, even when this browser's clock says
// otherwise. A reload of the list after the refusal (the requirement was turned off,
// or the User enrolled) lifts it; a page still refused refuses again.
function useSecondFactorRefused(workspaceId: string, listedAt: number) {
  const queryCache = useQueryClient().getQueryCache()
  const [refused, setRefused] = useState<{ workspaceId: string; at: number }>()
  useEffect(
    () =>
      queryCache.subscribe((event) => {
        if (event.type === 'updated' && isSecondFactorRequiredError(event.query.state.error)) {
          setRefused({ workspaceId, at: event.query.state.errorUpdatedAt })
        }
      }),
    [queryCache, workspaceId],
  )
  return refused?.workspaceId === workspaceId && refused.at >= listedAt
}

// WorkspaceLayout is the shell for workspace-scoped pages (overview, contacts,
// activity; workspace settings under its own sidebar): workspace sidebar + switcher. Under a Two-factor
// requirement it shows the grace banner, and once the grace is over it replaces
// the page with the screen that leads to enrollment.
export function WorkspaceLayout() {
  const { workspaceId } = workspaceRoute.useParams()
  // The settings area swaps the workspace sidebar for its own.
  const inSettings = Boolean(
    useMatchRoute()({ to: settingsRoute.to, params: { workspaceId }, fuzzy: true }),
  )
  const workspacesQuery = useQuery(siteWorkspacesListOptions())
  const workspaces = workspacesQuery.data ?? []
  const current = workspaces.find((w) => w.id === workspaceId)
  const holds = current?.holds ?? []
  const [now] = useState(() => Date.now())
  const refused = useSecondFactorRefused(workspaceId, workspacesQuery.dataUpdatedAt)
  const standing = refused ? { kind: 'blocked' as const } : secondFactorStanding(current, now)

  return (
    <DashboardShell
      sidebar={
        inSettings ? (
          <SettingsNavbar workspaceId={workspaceId} />
        ) : (
          <AppNavbar workspaceId={workspaceId} />
        )
      }
      banner={
        <>
          <LicenseBanner />
          <UpdateBanner />
          <InstanceNotice />
          {current?.suspendedAt ? (
            <SuspensionBanner
              reason={current.suspensionReason}
              supportEmail={current.supportEmail}
            />
          ) : null}
          {holds.length > 0 && current ? <HoldBanner holds={holds} workspace={current} /> : null}
          {standing.kind === 'grace' ? <SecondFactorGraceBanner endsAt={standing.endsAt} /> : null}
        </>
      }
      content={
        standing.kind === 'blocked' ? <SecondFactorBlocked workspace={current} /> : undefined
      }
      headerRight={
        <Group gap="sm" wrap="nowrap" flex={1} miw={0} justify="flex-end">
          <WorkspaceSwitcher workspaceId={workspaceId} workspaces={workspaces} />
          <UserMenu
            workspaceId={workspaceId}
            accountLink={current?.accountLinkAvailable ?? false}
          />
        </Group>
      }
    />
  )
}
