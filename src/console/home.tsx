import { Stack, Text, Title } from '@mantine/core'
import { useRouteContext } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { Route as consoleAuthedRoute } from '../routes/console/_authed/route.tsx'

// The console's home, on every Instance: who is signed in. The sections live in the navbar.
export function ConsoleHomePage() {
  const { t } = useTranslation()
  const { staff } = useRouteContext({ from: consoleAuthedRoute.id })

  return (
    <Stack>
      <Title order={2}>{t(($) => $.console.home.title)}</Title>
      <Text c="dimmed">{t(($) => $.console.home.signedInAs, { email: staff.email })}</Text>
    </Stack>
  )
}
