import { Badge, Button, Card, Group, Stack, Text, ThemeIcon, Title } from '@mantine/core'
import { IconCheck, IconCircle, IconClock } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'

import { ButtonLink } from '../../components/RouterLink.tsx'
import type { SiteSetupProgress, SiteSetupStepStatus } from '../../generated/site/types.gen.ts'
import { useDismissSetupChecklist } from '../../hooks/useDismissSetupChecklist.ts'
import { Route as broadcastsCreateRoute } from '../../routes/workspaces/$slug/broadcasts/new.tsx'
import { Route as contactsRoute } from '../../routes/workspaces/$slug/contacts/index.tsx'
import { Route as settingsRoute } from '../../routes/workspaces/$slug/settings.tsx'

type StepKey = keyof SiteSetupProgress['steps']

const STEP_ORDER: StepKey[] = ['integration', 'sendingDomain', 'contact', 'sentEmail']

function StatusIcon({ status }: { status: SiteSetupStepStatus }) {
  if (status === 'done') {
    return (
      <ThemeIcon color="teal" radius="xl" size="md">
        <IconCheck size={14} />
      </ThemeIcon>
    )
  }
  if (status === 'pending') {
    return (
      <ThemeIcon color="yellow" radius="xl" size="md">
        <IconClock size={14} />
      </ThemeIcon>
    )
  }
  return (
    <ThemeIcon color="gray" variant="light" radius="xl" size="md">
      <IconCircle size={14} />
    </ThemeIcon>
  )
}

// Each step links to the existing page where it is done.
const STEP_TARGET = {
  integration: settingsRoute.to,
  sendingDomain: settingsRoute.to,
  contact: contactsRoute.to,
  sentEmail: broadcastsCreateRoute.to,
} as const

function StepLink({ step, slug }: { step: StepKey; slug: string }) {
  const { t } = useTranslation()
  return (
    <ButtonLink to={STEP_TARGET[step]} params={{ slug }} variant="default" size="xs">
      {t(($) => $.setupChecklist.steps[step].action)}
    </ButtonLink>
  )
}

// SetupChecklist shows the four steps of the Workspace's path to its first sent email, each
// with its status as the server read it and a link to the page where it is done. No step
// blocks another, so every undone step keeps its link.
export function SetupChecklist({
  slug,
  checklist,
  canDismiss,
}: {
  slug: string
  checklist: SiteSetupProgress
  canDismiss: boolean
}) {
  const { t } = useTranslation()
  const dismiss = useDismissSetupChecklist(slug)
  return (
    <Card withBorder padding="lg">
      <Stack gap="md">
        <Group justify="space-between" align="flex-start" wrap="wrap" gap="sm">
          <Stack gap={2}>
            <Title order={3}>{t(($) => $.setupChecklist.title)}</Title>
            <Text size="sm" c="dimmed">
              {t(($) => $.setupChecklist.description)}
            </Text>
          </Stack>
          {canDismiss && (
            <Button
              variant="subtle"
              color="gray"
              size="xs"
              loading={dismiss.isPending}
              onClick={() => dismiss.mutate({ path: { slug } })}
            >
              {t(($) => $.setupChecklist.dismiss)}
            </Button>
          )}
        </Group>
        {STEP_ORDER.map((step) => {
          const status = checklist.steps[step]
          return (
            <Group key={step} justify="space-between" align="center" wrap="wrap" gap="sm">
              <Group gap="sm" wrap="nowrap" align="flex-start" flex={1} miw={220}>
                <StatusIcon status={status} />
                <Stack gap={0}>
                  <Group gap="xs">
                    <Text fw={500}>{t(($) => $.setupChecklist.steps[step].title)}</Text>
                    {status === 'pending' && (
                      <Badge color="yellow" variant="light">
                        {t(($) => $.setupChecklist.waitingForDns)}
                      </Badge>
                    )}
                    {status === 'done' && (
                      <Badge color="teal" variant="light">
                        {t(($) => $.setupChecklist.done)}
                      </Badge>
                    )}
                  </Group>
                  <Text size="sm" c="dimmed">
                    {t(($) => $.setupChecklist.steps[step].description)}
                  </Text>
                </Stack>
              </Group>
              {status !== 'done' && <StepLink step={step} slug={slug} />}
            </Group>
          )
        })}
      </Stack>
    </Card>
  )
}
