import { Alert, Anchor, Group, Select, Text, Tooltip } from '@mantine/core'
import { IconCreditCard, IconPlayerPause, IconPlus } from '@tabler/icons-react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useMatchRoute, useNavigate } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { AppNavbar } from '../components/AppNavbar.tsx'
import { InstanceNotice } from '../components/InstanceNotice.tsx'
import { LicenseBanner } from '../components/LicenseBanner.tsx'
import { ActionIconLink } from '../components/RouterLink.tsx'
import {
  SecondFactorBlocked,
  SecondFactorGraceBanner,
  secondFactorStanding,
} from '../components/SecondFactorNotice.tsx'
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
import { DashboardShell } from './DashboardShell.tsx'

// WorkspaceSwitcher changes the active workspace by navigating to its
// /w/{workspaceId} route.
function WorkspaceSwitcher({
  workspaceId,
  workspaces,
}: {
  workspaceId: string
  workspaces: SiteWorkspaceResource[]
}) {
  const navigate = useNavigate()
  const data = workspaces.map((w) => ({ value: w.id, label: w.name }))

  const { t } = useTranslation()

  return (
    <Group gap="xs" wrap="nowrap">
      <Select
        data={data}
        value={workspaceId}
        allowDeselect={false}
        onChange={(value) => {
          if (value && value !== workspaceId) {
            void navigate({ to: overviewRoute.to, params: { workspaceId: value } })
          }
        }}
        w={{ base: 140, sm: 220 }}
      />
      <Tooltip label={t(($) => $.createWorkspace.switcherLabel)}>
        <ActionIconLink
          to={newWorkspaceRoute.to}
          variant="default"
          size="lg"
          aria-label={t(($) => $.createWorkspace.switcherLabel)}
        >
          <IconPlus size={18} />
        </ActionIconLink>
      </Tooltip>
    </Group>
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

// BillingLink points an Owner or Admin to the configured billing site, and tells a plain
// member to ask one of them; with no billing site configured it renders nothing.
function BillingLink({
  role,
  billingUrl,
}: {
  role: SiteWorkspaceResource['role']
  billingUrl: string | null | undefined
}) {
  const { t } = useTranslation()
  if (!billingUrl) {
    return null
  }
  return role === 'member' ? (
    <Text size="sm" mt="xs">
      {t(($) => $.workspaceHold.billingContact)}
    </Text>
  ) : (
    <Anchor size="sm" mt="xs" display="block" href={billingUrl}>
      {t(($) => $.workspaceHold.billingLink)}
    </Anchor>
  )
}

// HoldBanner tells every member that outbound sending is held (ADR 0009) and shows each
// setter's message as given. It is deliberately not the suspension banner: another color
// and icon, another title. A billing hold (paid-for, not abuse) has its own color, icon
// and title so nobody reads a payment problem as an abuse flag, and only it points to
// the billing site.
function HoldBanner({
  variant,
  holds,
  role,
  billingUrl,
}: {
  variant: 'generic' | 'billing'
  holds: SiteWorkspaceHold[]
  role: SiteWorkspaceResource['role']
  billingUrl: string | null | undefined
}) {
  const { t } = useTranslation()
  const billing = variant === 'billing'
  return (
    <Alert
      color={billing ? 'orange' : 'yellow'}
      variant="outline"
      icon={billing ? <IconCreditCard /> : <IconPlayerPause />}
      title={billing ? t(($) => $.workspaceBillingHold.title) : t(($) => $.workspaceHold.title)}
      mb="md"
    >
      <Text size="sm">
        {billing
          ? t(($) => $.workspaceBillingHold.description)
          : t(($) => $.workspaceHold.description)}
      </Text>
      {holds.map((hold) => (
        <Text key={hold.key} size="sm" fw={500} mt="xs">
          {hold.message}
        </Text>
      ))}
      {billing ? <BillingLink role={role} billingUrl={billingUrl} /> : null}
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
  const billingHolds = holds.filter((hold) => hold.billing)
  const otherHolds = holds.filter((hold) => !hold.billing)
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
          {otherHolds.length > 0 && current ? (
            <HoldBanner
              variant="generic"
              holds={otherHolds}
              role={current.role}
              billingUrl={current.billingUrl}
            />
          ) : null}
          {billingHolds.length > 0 && current ? (
            <HoldBanner
              variant="billing"
              holds={billingHolds}
              role={current.role}
              billingUrl={current.billingUrl}
            />
          ) : null}
          {standing.kind === 'grace' ? <SecondFactorGraceBanner endsAt={standing.endsAt} /> : null}
        </>
      }
      content={
        standing.kind === 'blocked' ? <SecondFactorBlocked workspace={current} /> : undefined
      }
      headerRight={
        <Group gap="sm">
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
