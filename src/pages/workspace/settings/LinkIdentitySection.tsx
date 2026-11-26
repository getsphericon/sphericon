import { Button, Group, Stack, Text, Title } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { useTranslation } from 'react-i18next'

import type { SiteWorkspaceResource } from '../../../generated/site/types.gen.ts'
import { can } from '../../../utils/permissions.ts'
import { linkIdentityEditor } from './workspaceSettingsEditors.ts'

// LinkIdentitySection shows the Workspace's Link identity tokens (ADR 0160): whether the click of a
// tracked link to one of its own sites carries the recipient's token, which the tracker there turns
// into the binding of the anonymous visitor. Every member sees the setting; an Owner or Admin
// changes it in a modal, an editor session over the Workspace's revision (ADR 0119).
export function LinkIdentitySection({ workspace }: { workspace: SiteWorkspaceResource }) {
  const { t } = useTranslation()
  const [opened, { open, close }] = useDisclosure(false)
  const hosts = workspace.linkIdentityHosts.join(', ')

  return (
    <Stack gap="xs">
      <Title order={4}>{t(($) => $.settings.linkIdentity.title)}</Title>
      <Text size="sm" c="dimmed">
        {t(($) => $.settings.linkIdentity.description)}
      </Text>
      <Group gap="xs">
        <Text size="sm">
          {workspace.linkIdentityEnabled
            ? t(($) => $.settings.linkIdentity.on, { hosts })
            : t(($) => $.settings.linkIdentity.off)}
        </Text>
        {can(workspace, 'workspace:update') ? (
          <Button variant="subtle" size="compact-sm" onClick={open}>
            {t(($) => $.settings.linkIdentity.change)}
          </Button>
        ) : null}
      </Group>
      <linkIdentityEditor.EditModal id={opened ? 'link-identity' : null} onClose={close} />
    </Stack>
  )
}
