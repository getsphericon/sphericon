import { Alert, Anchor, Group, Select, Text } from '@mantine/core'
import { IconPlayerPause } from '@tabler/icons-react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useMatchRoute, useNavigate } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { AppNavbar } from '../components/AppNavbar.tsx'
import { InstanceNotice } from '../components/InstanceNotice.tsx'
import { LicenseBanner } from '../components/LicenseBanner.tsx'
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
import { Route as overviewRoute } from '../routes/workspaces/$slug/index.tsx'
import { Route as workspaceRoute } from '../routes/workspaces/$slug/route.tsx'
import { Route as settingsRoute } from '../routes/workspaces/$slug/settings/route.tsx'
import { isSecondFactorRequiredError } from '../utils/apiErrors.ts'
import { DashboardShell } from './DashboardShell.tsx'

// WorkspaceSwitcher changes the active workspace by navigating to its
// /w/{slug} route.
function WorkspaceSwitcher({
  slug,
  workspaces,
}: {
  slug: string
  workspaces: SiteWorkspaceResource[]
}) {
  const navigate = useNavigate()
  const data = workspaces.map((w) => ({ value: w.slug, label: w.name }))

  return (
    <Select
      data={data}
      value={slug}
      allowDeselect={false}
      onChange={(value) => {
        if (value && value !== slug) {
          void navigate({ to: overviewRoute.to, params: { slug: value } })
        }
      }}
      w={{ base: 140, sm: 220 }}
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

// HoldAction points an Owner or Admin to where the setter of a hold says it can be resolved,
// and tells a plain member to ask one of them; a hold with no action URL renders nothing.
function HoldAction({
  role,
  actionUrl,
}: {
  role: SiteWorkspaceResource['role']
  actionUrl: string | null | undefined
}) {
  const { t } = useTranslation()
  if (!actionUrl) {
    return null
  }
  return role === 'member' ? (
    <Text size="sm">{t(($) => $.workspaceHold.actionContact)}</Text>
  ) : (
    <Anchor size="sm" display="block" href={actionUrl}>
      {t(($) => $.workspaceHold.actionLink)}
    </Anchor>
  )
}

// HoldBanner tells every member that outbound sending is held (ADR 0009) and shows each
// setter's message as given, with the link to resolve it when the setter gave one. It is
// deliberately not the suspension banner: another color and icon, another title.
function HoldBanner({
  holds,
  role,
}: {
  holds: SiteWorkspaceHold[]
  role: SiteWorkspaceResource['role']
}) {
  const { t } = useTranslation()
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
          <HoldAction role={role} actionUrl={hold.actionUrl} />
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
function useSecondFactorRefused(slug: string, listedAt: number) {
  const queryCache = useQueryClient().getQueryCache()
  const [refused, setRefused] = useState<{ slug: string; at: number }>()
  useEffect(
    () =>
      queryCache.subscribe((event) => {
        if (event.type === 'updated' && isSecondFactorRequiredError(event.query.state.error)) {
          setRefused({ slug, at: event.query.state.errorUpdatedAt })
        }
      }),
    [queryCache, slug],
  )
  return refused?.slug === slug && refused.at >= listedAt
}

// WorkspaceLayout is the shell for workspace-scoped pages (overview, contacts,
// activity; workspace settings under its own sidebar): workspace sidebar + switcher. Under a Two-factor
// requirement it shows the grace banner, and once the grace is over it replaces
// the page with the screen that leads to enrollment.
export function WorkspaceLayout() {
  const { slug } = workspaceRoute.useParams()
  // The settings area swaps the workspace sidebar for its own.
  const inSettings = Boolean(
    useMatchRoute()({ to: settingsRoute.to, params: { slug }, fuzzy: true }),
  )
  const workspacesQuery = useQuery(siteWorkspacesListOptions())
  const workspaces = workspacesQuery.data ?? []
  const current = workspaces.find((w) => w.slug === slug)
  const holds = current?.holds ?? []
  const [now] = useState(() => Date.now())
  const refused = useSecondFactorRefused(slug, workspacesQuery.dataUpdatedAt)
  const standing = refused ? { kind: 'blocked' as const } : secondFactorStanding(current, now)

  return (
    <DashboardShell
      sidebar={inSettings ? <SettingsNavbar slug={slug} /> : <AppNavbar slug={slug} />}
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
          {holds.length > 0 && current ? <HoldBanner holds={holds} role={current.role} /> : null}
          {standing.kind === 'grace' ? <SecondFactorGraceBanner endsAt={standing.endsAt} /> : null}
        </>
      }
      content={
        standing.kind === 'blocked' ? <SecondFactorBlocked workspace={current} /> : undefined
      }
      headerRight={
        <Group gap="sm">
          <WorkspaceSwitcher slug={slug} workspaces={workspaces} />
          <UserMenu slug={slug} accountLink={current?.accountLinkAvailable ?? false} />
        </Group>
      }
    />
  )
}
