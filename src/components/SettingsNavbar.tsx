import { Stack } from '@mantine/core'
import {
  IconArrowLeft,
  IconHistory,
  IconKey,
  IconLock,
  IconMailForward,
  IconRadar,
  IconRobot,
  IconSettings,
  IconUsers,
  IconWebhook,
} from '@tabler/icons-react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { siteWorkspacesListOptions } from '../generated/site/@tanstack/react-query.gen.ts'
import { Route as overviewRoute } from '../routes/workspaces/$workspaceId/index.tsx'
import { Route as apiKeysRoute } from '../routes/workspaces/$workspaceId/settings/api-keys.tsx'
import { Route as auditLogRoute } from '../routes/workspaces/$workspaceId/settings/audit-log.tsx'
import { Route as emailsRoute } from '../routes/workspaces/$workspaceId/settings/emails.tsx'
import { Route as generalRoute } from '../routes/workspaces/$workspaceId/settings/index.tsx'
import { Route as mcpRoute } from '../routes/workspaces/$workspaceId/settings/mcp.tsx'
import { Route as privacyRoute } from '../routes/workspaces/$workspaceId/settings/privacy.tsx'
import { Route as teamRoute } from '../routes/workspaces/$workspaceId/settings/team.tsx'
import { Route as trackingRoute } from '../routes/workspaces/$workspaceId/settings/tracking.tsx'
import { Route as webhooksRoute } from '../routes/workspaces/$workspaceId/settings/webhooks.tsx'
import { can } from '../utils/permissions.ts'
import { NavLinkLink } from './RouterLink.tsx'

// SettingsNavbar is the sidebar of the workspace settings area: a way back to the
// workspace plus one link per settings page. General is the area's index, so it is
// current only on itself.
export function SettingsNavbar({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation()
  const workspaces = useQuery(siteWorkspacesListOptions())
  const workspace = workspaces.data?.find((w) => w.id === workspaceId)

  const pages = [
    {
      route: generalRoute,
      label: t(($) => $.settings.nav.general),
      icon: <IconSettings size={18} />,
      exact: true,
    },
    {
      route: teamRoute,
      label: t(($) => $.settings.nav.team),
      icon: <IconUsers size={18} />,
      exact: false,
    },
    {
      route: auditLogRoute,
      label: t(($) => $.settings.nav.auditLog),
      icon: <IconHistory size={18} />,
      exact: false,
    },
    {
      route: trackingRoute,
      label: t(($) => $.settings.nav.tracking),
      icon: <IconRadar size={18} />,
      exact: false,
    },
    {
      route: emailsRoute,
      label: t(($) => $.settings.nav.emails),
      icon: <IconMailForward size={18} />,
      exact: false,
    },
    {
      route: apiKeysRoute,
      label: t(($) => $.settings.nav.apiKeys),
      icon: <IconKey size={18} />,
      exact: false,
    },
    // Webhooks and Privacy requests are offered only to a member who holds their Permission;
    // the API refuses everyone else.
    ...(can(workspace, 'webhooks:manage')
      ? [
          {
            route: webhooksRoute,
            label: t(($) => $.settings.nav.webhooks),
            icon: <IconWebhook size={18} />,
            exact: false,
          },
        ]
      : []),
    {
      route: mcpRoute,
      label: t(($) => $.settings.nav.mcp),
      icon: <IconRobot size={18} />,
      exact: false,
    },
    ...(can(workspace, 'contacts:erase')
      ? [
          {
            route: privacyRoute,
            label: t(($) => $.settings.nav.privacy),
            icon: <IconLock size={18} />,
            exact: false,
          },
        ]
      : []),
  ]

  return (
    <Stack gap="xs">
      <NavLinkLink
        to={overviewRoute.to}
        params={{ workspaceId }}
        label={t(($) => $.settings.backToWorkspace)}
        leftSection={<IconArrowLeft size={18} />}
        activeOptions={{ exact: true }}
      />
      {pages.map(({ route, label, icon, exact }) => (
        <NavLinkLink
          key={route.to}
          to={route.to}
          params={{ workspaceId }}
          label={label}
          leftSection={icon}
          activeOptions={{ exact }}
        />
      ))}
    </Stack>
  )
}
