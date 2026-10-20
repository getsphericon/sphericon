import { Stack } from '@mantine/core'
import {
  IconArrowLeft,
  IconCode,
  IconMailForward,
  IconRadar,
  IconRobot,
  IconSettings,
  IconUsers,
} from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'

import { Route as overviewRoute } from '../routes/workspaces/$workspaceId/index.tsx'
import { Route as developersRoute } from '../routes/workspaces/$workspaceId/settings/developers.tsx'
import { Route as generalRoute } from '../routes/workspaces/$workspaceId/settings/index.tsx'
import { Route as mcpRoute } from '../routes/workspaces/$workspaceId/settings/mcp.tsx'
import { Route as sendingRoute } from '../routes/workspaces/$workspaceId/settings/sending.tsx'
import { Route as teamRoute } from '../routes/workspaces/$workspaceId/settings/team.tsx'
import { Route as trackingRoute } from '../routes/workspaces/$workspaceId/settings/tracking.tsx'
import { NavLinkLink } from './RouterLink.tsx'

// SettingsNavbar is the sidebar of the workspace settings area: a way back to the
// workspace plus one link per settings page. General is the area's index, so it is
// current only on itself.
export function SettingsNavbar({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation()

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
      route: trackingRoute,
      label: t(($) => $.settings.nav.tracking),
      icon: <IconRadar size={18} />,
      exact: false,
    },
    {
      route: sendingRoute,
      label: t(($) => $.settings.nav.sending),
      icon: <IconMailForward size={18} />,
      exact: false,
    },
    {
      route: developersRoute,
      label: t(($) => $.settings.nav.developers),
      icon: <IconCode size={18} />,
      exact: false,
    },
    {
      route: mcpRoute,
      label: t(($) => $.settings.nav.mcp),
      icon: <IconRobot size={18} />,
      exact: false,
    },
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
