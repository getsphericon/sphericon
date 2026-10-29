import { Badge, Card, Grid, Group, Stack, Text, ThemeIcon, Title } from '@mantine/core'
import { IconCheck, IconCircle, IconClock } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'

import { ButtonLink } from '../../components/RouterLink.tsx'
import type { SiteSetupProgress, SiteSetupStepStatus } from '../../generated/site/types.gen.ts'
import { Route as automationsRoute } from '../../routes/workspaces/$workspaceId/automations/index.tsx'
import { Route as broadcastsCreateRoute } from '../../routes/workspaces/$workspaceId/broadcasts/new.tsx'
import { Route as contactsRoute } from '../../routes/workspaces/$workspaceId/contacts/index.tsx'
import { Route as apiKeysRoute } from '../../routes/workspaces/$workspaceId/settings/api-keys.tsx'
import { Route as emailsRoute } from '../../routes/workspaces/$workspaceId/settings/emails.tsx'
import { Route as trackingRoute } from '../../routes/workspaces/$workspaceId/settings/tracking.tsx'

type StepKey = keyof SiteSetupProgress['steps']
type LinkedStepKey = Exclude<StepKey, 'contact' | 'sentEmail'>

// The steps in the order the checklist shows them, each with the existing page where it is done.
// A step with no single page (`to: null`) renders its own actions.
const STEPS = [
  { key: 'integration', to: emailsRoute.to },
  { key: 'sendingDomain', to: emailsRoute.to },
  { key: 'contact', to: null },
  { key: 'sentEmail', to: null },
] as const satisfies readonly { key: StepKey; to: string | null }[]

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
  workspaceId,
}: {
  step: LinkedStepKey
  to: string
  workspaceId: string
}) {
  const { t } = useTranslation()
  return (
    <ButtonLink to={to} params={{ workspaceId }} variant="default" size="xs">
      {t(($) => $.setupChecklist.steps[step].action)}
    </ButtonLink>
  )
}

// ContactActions offers every way a Contact gets into the Workspace: any one of them completes the
// step, which the server reads off the Contacts themselves, so none is marked here.
function ContactActions({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation()
  return (
    <Group gap="xs" wrap="wrap">
      <ButtonLink to={trackingRoute.to} params={{ workspaceId }} variant="default" size="xs">
        {t(($) => $.setupChecklist.steps.contact.actions.tracker)}
      </ButtonLink>
      <ButtonLink to={apiKeysRoute.to} params={{ workspaceId }} variant="default" size="xs">
        {t(($) => $.setupChecklist.steps.contact.actions.api)}
      </ButtonLink>
      <ButtonLink to={contactsRoute.to} params={{ workspaceId }} variant="default" size="xs">
        {t(($) => $.setupChecklist.steps.contact.actions.manual)}
      </ButtonLink>
    </Group>
  )
}

// SendActions offers every surface that sends email: a real send from any of them completes the
// step, which the server reads off the Outbound messages themselves.
function SendActions({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation()
  return (
    <Group gap="xs" wrap="wrap">
      <ButtonLink
        to={broadcastsCreateRoute.to}
        params={{ workspaceId }}
        variant="default"
        size="xs"
      >
        {t(($) => $.setupChecklist.steps.sentEmail.actions.broadcast)}
      </ButtonLink>
      <ButtonLink to={automationsRoute.to} params={{ workspaceId }} variant="default" size="xs">
        {t(($) => $.setupChecklist.steps.sentEmail.actions.automation)}
      </ButtonLink>
    </Group>
  )
}

// SetupChecklist shows the steps of the Workspace's path to its first sent email, each
// with its status as the server read it and a link to the page where it is done. No step
// blocks another, so every undone step keeps its link.
export function SetupChecklist({
  workspaceId,
  checklist,
}: {
  workspaceId: string
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
            <Grid key={step} type="container" align="center" gap="sm">
              <Grid.Col span={{ base: 12, sm: 7 }}>
                <Group gap="sm" wrap="nowrap" align="flex-start">
                  <StatusIcon status={status} />
                  <Stack gap={0} flex={1} miw={0}>
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
              </Grid.Col>
              <Grid.Col span={{ base: 12, sm: 5 }}>
                {status !== 'done' &&
                  (to === null ? (
                    step === 'contact' ? (
                      <ContactActions workspaceId={workspaceId} />
                    ) : (
                      <SendActions workspaceId={workspaceId} />
                    )
                  ) : (
                    <StepLink step={step} to={to} workspaceId={workspaceId} />
                  ))}
              </Grid.Col>
            </Grid>
          )
        })}
      </Stack>
    </Card>
  )
}
