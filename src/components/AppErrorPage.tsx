import { Button, Stack, Text, Title } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { getApiErrorMessage } from '../utils/apiErrors.ts'

interface AppErrorPageProps {
  error: unknown
}

// AppErrorPage is what the router renders for an error no route handled. A failed API call
// carries a problem body: its detail is shown (a server that is not ready names what to run,
// e.g. the migrate command), so the page never stops at a bare "something went wrong".
export function AppErrorPage({ error }: AppErrorPageProps) {
  const { t } = useTranslation()

  return (
    <Stack align="center" maw={520} mx="auto" mt={120} gap="xs" ta="center">
      <Title order={2}>{t(($) => $.appError.title)}</Title>
      <Text c="dimmed">
        {getApiErrorMessage(
          error,
          t(($) => $.appError.fallback),
        )}
      </Text>
      <Button
        mt="md"
        onClick={() => {
          globalThis.location.reload()
        }}
      >
        {t(($) => $.appError.reload)}
      </Button>
    </Stack>
  )
}
