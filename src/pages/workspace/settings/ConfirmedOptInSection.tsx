import { Stack, Switch, Text, Title } from '@mantine/core'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { QueryBoundary } from '../../../components/QueryBoundary.tsx'
import { BlockSkeleton } from '../../../components/Skeletons.tsx'
import { siteWorkspacesGetConfirmationEmailOptions } from '../../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteWorkspaceResource } from '../../../generated/site/types.gen.ts'
import { useConfirmation } from '../../../hooks/useConfirmation.tsx'
import { useSetConfirmedOptIn } from '../../../hooks/useConfirmedOptIn.ts'
import { can } from '../../../utils/permissions.ts'
import { ConfirmationEmailEditForm } from './workspaceSettingsEditors.ts'
import { CanEdit } from './workspaceSettingsForm.ts'

// The switch and the confirmation email it needs, for an Owner or Admin: turning it on asks them to
// affirm that the list they can email now consented, which is what grandfathers it (ADR 0013).
function ConfirmedOptInSettings({ workspace }: { workspace: SiteWorkspaceResource }) {
  const { t } = useTranslation()
  const confirm = useConfirmation()
  const mutation = useSetConfirmedOptIn(workspace.id)
  const { data: email } = useSuspenseQuery(
    siteWorkspacesGetConfirmationEmailOptions({ path: { workspaceId: workspace.id } }),
  )
  const setUp = email.fromEmail !== null
  const enabled = workspace.requireConfirmedOptIn
  const set = (on: boolean) =>
    mutation.mutate({
      path: { workspaceId: workspace.id },
      body: { revision: workspace.revision, enabled: on },
    })

  return (
    <Stack gap="lg">
      <Stack gap="xs">
        <Switch
          label={t(($) => $.settings.confirmedOptIn.switchLabel)}
          checked={enabled}
          // Turning it off is always offered; on needs the confirmation email (409 otherwise).
          disabled={mutation.isPending || (!enabled && !setUp)}
          onChange={(event) => {
            if (!event.currentTarget.checked) {
              set(false)
              return
            }
            confirm({
              title: t(($) => $.settings.confirmedOptIn.affirmTitle),
              description: t(($) => $.settings.confirmedOptIn.affirmDescription),
              confirmLabel: t(($) => $.settings.confirmedOptIn.affirmConfirm),
              onConfirm: () => set(true),
            })
          }}
        />
        {!enabled && !setUp ? (
          <Text size="sm" c="dimmed">
            {t(($) => $.settings.confirmedOptIn.emailRequired)}
          </Text>
        ) : null}
      </Stack>
      <Stack gap="xs">
        <Title order={5}>{t(($) => $.settings.confirmedOptIn.emailTitle)}</Title>
        <Text size="sm" c="dimmed">
          {t(($) => $.settings.confirmedOptIn.emailDescription)}
        </Text>
        <CanEdit value>
          <ConfirmationEmailEditForm
            workspaceId={workspace.id}
            id="confirmation-email"
            loaded={email}
            titled={false}
          />
        </CanEdit>
      </Stack>
    </Stack>
  )
}

// ConfirmedOptInSection holds the Workspace's confirmed opt-in policy (ADR 0013). Every member sees
// whether it is on; an Owner or Admin turns it on or off and sets up the confirmation email.
export function ConfirmedOptInSection({ workspace }: { workspace: SiteWorkspaceResource }) {
  const { t } = useTranslation()
  return (
    <Stack gap="xs">
      <Title order={4}>{t(($) => $.settings.confirmedOptIn.title)}</Title>
      <Text size="sm" c="dimmed">
        {t(($) => $.settings.confirmedOptIn.description)}
      </Text>
      {can(workspace, 'workspace:update') ? (
        <QueryBoundary
          fallback={<BlockSkeleton />}
          errorTitle={t(($) => $.settings.confirmedOptIn.loadError)}
        >
          <ConfirmedOptInSettings workspace={workspace} />
        </QueryBoundary>
      ) : (
        <Text size="sm">
          {workspace.requireConfirmedOptIn
            ? t(($) => $.settings.confirmedOptIn.on)
            : t(($) => $.settings.confirmedOptIn.off)}
        </Text>
      )}
    </Stack>
  )
}
