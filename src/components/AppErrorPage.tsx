import {
  Button,
  Code,
  CopyButton,
  Group,
  Stack,
  Text,
  ThemeIcon,
  Title,
  Tooltip,
  UnstyledButton,
} from '@mantine/core'
import { IconAlertTriangle, IconDatabaseImport, IconRefresh } from '@tabler/icons-react'
import { useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { siteSchemaApplyMigrationsMutation } from '../generated/site/@tanstack/react-query.gen.ts'
import {
  getApiErrorMessage,
  getProblemDebug,
  getProblemStatusLine,
  isMigrationsPendingError,
} from '../utils/apiErrors.ts'
import { splitInlineCode } from '../utils/inlineCode.ts'
import { ApiErrorAlert } from './ApiErrorAlert.tsx'
import { CenteredLayout } from './CenteredLayout.tsx'
import { ProblemDebugPanel } from './ProblemDebugPanel.tsx'

interface AppErrorPageProps {
  error: unknown
}

// AppErrorPage is what the router renders for an error no route handled. A failed API call
// carries a problem body: its detail is shown (a server that is not ready names what to run,
// e.g. the migrate command), so the page never stops at a bare "something went wrong". The
// commands a detail sets in backticks are copied with a click. A development server whose
// database is behind (migrations_pending, ADR 0033) gets a button that runs the migrations.
export function AppErrorPage({ error }: AppErrorPageProps) {
  const { t } = useTranslation()
  const migrate = useMutation({
    ...siteSchemaApplyMigrationsMutation(),
    onSuccess: () => {
      globalThis.location.reload()
    },
  })
  const pending = isMigrationsPendingError(error)
  const statusLine = getProblemStatusLine(error)
  const message = getApiErrorMessage(
    error,
    t(($) => $.appError.fallback),
  )

  return (
    <CenteredLayout maw={getProblemDebug(error) === undefined ? 520 : 900} align="center">
      <ThemeIcon variant="light" color="red" size={64} radius="xl">
        <IconAlertTriangle size={34} stroke={1.5} />
      </ThemeIcon>
      <Stack gap={4} align="center" ta="center">
        {statusLine === undefined ? null : (
          <Text size="sm" c="dimmed" ff="monospace">
            {statusLine}
          </Text>
        )}
        <Title order={2}>{t(($) => $.appError.title)}</Title>
      </Stack>
      <Text c="dimmed" ta="center" maw={520}>
        {splitInlineCode(message).map((span, index) =>
          span.code ? <CopyCommand key={index} command={span.text} /> : span.text,
        )}
      </Text>
      <ProblemDebugPanel error={error} />
      {migrate.isError ? (
        <ApiErrorAlert
          error={migrate.error}
          title={t(($) => $.appError.migrateErrorTitle)}
          fallback={t(($) => $.appError.migrateErrorBody)}
        />
      ) : null}
      <Group mt="sm" justify="center">
        {pending ? (
          <Button
            leftSection={<IconDatabaseImport size={16} />}
            loading={migrate.isPending}
            onClick={() => {
              migrate.mutate({})
            }}
          >
            {t(($) => $.appError.migrate)}
          </Button>
        ) : null}
        <Button
          variant={pending ? 'default' : 'filled'}
          leftSection={<IconRefresh size={16} />}
          disabled={migrate.isPending}
          onClick={() => {
            globalThis.location.reload()
          }}
        >
          {t(($) => $.appError.reload)}
        </Button>
      </Group>
    </CenteredLayout>
  )
}

// CopyCommand is a command inside the message: set as code, copied to the clipboard on a click.
function CopyCommand({ command }: { command: string }) {
  const { t } = useTranslation()
  return (
    <CopyButton value={command}>
      {({ copied, copy }) => (
        <Tooltip
          label={copied ? t(($) => $.appError.copied) : t(($) => $.appError.copyHint)}
          withArrow
        >
          <UnstyledButton aria-label={t(($) => $.appError.copyCommand, { command })} onClick={copy}>
            <Code>{command}</Code>
          </UnstyledButton>
        </Tooltip>
      )}
    </CopyButton>
  )
}
