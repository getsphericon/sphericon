import { Badge, Card, Group, Stack, Text, ThemeIcon, Title } from '@mantine/core'
import { IconCheck, IconCircle, IconClock } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'

import { ButtonLink } from '../../components/RouterLink.tsx'
import type { SiteSetupProgress, SiteSetupStepStatus } from '../../generated/site/types.gen.ts'
import { Route as broadcastsCreateRoute } from '../../routes/workspaces/$slug/broadcasts/new.tsx'
import { Route as contactsRoute } from '../../routes/workspaces/$slug/contacts/index.tsx'
import { Route as sendingRoute } from '../../routes/workspaces/$slug/settings/sending.tsx'
import { SetupDomainStep } from './SetupDomainStep.tsx'

type StepKey = keyof SiteSetupProgress['steps']

// The steps in the order the checklist shows them, each with the existing page where it is done.
const STEPS = [
  { key: 'integration', to: sendingRoute.to },
  { key: 'sendingDomain', to: sendingRoute.to },
  { key: 'contact', to: contactsRoute.to },
  { key: 'sentEmail', to: broadcastsCreateRoute.to },
] as const satisfies readonly { key: StepKey; to: string }[]

const STATUS_ICON = {
  done: { color: 'teal', variant: 'filled', Icon: IconCheck },
  pending: { color: 'yellow', variant: 'filled', Icon: IconClock },
  todo: { color: 'gray', variant: 'light', Icon: IconCircle },
} as const satisfies Record<SiteSetupStepStatus, unknown>

function StatusIcon({ status }: { status: SiteSetupStepStatus }) {
  const { color, variant, Icon } = STATUS_ICON[status]
  return (
    <ThemeIcon color={color} variant={variant} radius="xl" size="md">
      <Icon size={14} />
    </ThemeIcon>
  )
}

function StepLink({
  step,
  to,
  slug,
}: {
  step: StepKey
  to: (typeof STEPS)[number]['to']
  slug: string
}) {
  const { t } = useTranslation()
  return (
    <ButtonLink to={to} params={{ slug }} variant="default" size="xs">
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
}: {
  slug: string
  checklist: SiteSetupProgress
}) {
  const { t } = useTranslation()
  return (
    <Card withBorder padding="lg">
      <Stack gap="md">
        <Stack gap={2}>
          <Title order={3}>{t(($) => $.setupChecklist.title)}</Title>
          <Text size="sm" c="dimmed">
            {t(($) => $.setupChecklist.description)}
          </Text>
        </Stack>
        {STEPS.map(({ key: step, to }) => {
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
                  {step === 'sendingDomain' && status === 'pending' && (
                    <SetupDomainStep slug={slug} />
                  )}
                </Stack>
              </Group>
              {status !== 'done' && <StepLink step={step} to={to} slug={slug} />}
            </Group>
          )
        })}
      </Stack>
    </Card>
  )
}
