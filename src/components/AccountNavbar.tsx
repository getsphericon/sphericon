import { NavLink, Stack } from '@mantine/core'
import { IconArrowLeft, IconShieldLock, IconUser } from '@tabler/icons-react'
import { useMatchRoute, useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { Route as profileRoute } from '../routes/account/index.tsx'
import { Route as securityRoute } from '../routes/account/security.tsx'
import { Route as indexRoute } from '../routes/index.tsx'

// AccountNavbar is the sidebar for the workspace-independent account area:
// a way back to the workspace dashboard plus account sections.
export function AccountNavbar() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const matchRoute = useMatchRoute()

  return (
    <Stack gap="xs">
      <NavLink
        label={t(($) => $.account.backToDashboard)}
        leftSection={<IconArrowLeft size={18} />}
        onClick={() => void navigate({ to: indexRoute.to })}
      />
      <NavLink
        label={t(($) => $.account.profile)}
        leftSection={<IconUser size={18} />}
        active={Boolean(matchRoute({ to: profileRoute.to }))}
        onClick={() => void navigate({ to: profileRoute.to })}
      />
      <NavLink
        label={t(($) => $.account.security)}
        leftSection={<IconShieldLock size={18} />}
        active={Boolean(matchRoute({ to: securityRoute.to }))}
        onClick={() => void navigate({ to: securityRoute.to })}
      />
    </Stack>
  )
}
