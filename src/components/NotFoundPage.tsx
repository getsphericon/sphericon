import { Stack, Text, Title } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { Route as indexRoute } from '../routes/index.tsx'
import { ButtonLink } from './RouterLink.tsx'

// NotFoundPage is what the router renders for an address no route matches.
export function NotFoundPage() {
  const { t } = useTranslation()

  return (
    <Stack align="center" maw={420} mx="auto" mt={120} gap="xs" ta="center">
      <Text c="dimmed" fw={700} fz={64} lh={1}>
        {'404'}
      </Text>
      <Title order={2}>{t(($) => $.notFound.title)}</Title>
      <Text c="dimmed">{t(($) => $.notFound.description)}</Text>
      <ButtonLink to={indexRoute.to} mt="md">
        {t(($) => $.notFound.home)}
      </ButtonLink>
    </Stack>
  )
}
