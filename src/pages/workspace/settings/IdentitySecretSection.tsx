import { Box, Button, Code, Group, Text, Title } from '@mantine/core'
import { modals } from '@mantine/modals'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { CopyTextButton } from '../../../components/CopyTextButton.tsx'
import {
  siteIdentitySecretGetOptions,
  siteIdentitySecretRegenerateMutation,
} from '../../../generated/site/@tanstack/react-query.gen.ts'
import { useResourceMutation } from '../../../hooks/useResourceMutation.ts'

// The identify a signed-in page sends: the token is what the site's server signs with the secret.
const IDENTIFY_SNIPPET = `_omq.push(['identify', { identityToken: '<the token your server signs>' }]);`

// IdentitySecretSection holds the Workspace's Identity secret (ADR 0171): the site's server signs an
// Identity token with it for the person signed in, and an identify that carries the token is proven.
// Owners and admins only, as the API refuses everyone else. Regenerating replaces the secret at once,
// so it is confirmed first.
export function IdentitySecretSection({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation()
  const query = useQuery(siteIdentitySecretGetOptions({ path: { workspaceId } }))
  const regenerate = useResourceMutation({
    mutation: siteIdentitySecretRegenerateMutation(),
    successMessage: t(($) => $.settings.identitySecret.generated),
    errorTitle: t(($) => $.settings.identitySecret.generateError),
  })
  const secret = query.data?.secret

  const onGenerate = () => {
    if (!secret) {
      regenerate.mutate({ path: { workspaceId } })
      return
    }
    modals.openConfirmModal({
      title: t(($) => $.settings.identitySecret.regenerateConfirmTitle),
      children: <Text>{t(($) => $.settings.identitySecret.regenerateConfirmBody)}</Text>,
      labels: {
        cancel: t(($) => $.actions.cancel),
        confirm: t(($) => $.settings.identitySecret.regenerate),
      },
      confirmProps: { color: 'red' },
      onConfirm: () => regenerate.mutate({ path: { workspaceId } }),
    })
  }

  return (
    <Box>
      <Title order={4} mb="xs">
        {t(($) => $.settings.identitySecret.title)}
      </Title>
      <Text c="dimmed" size="sm" mb="sm">
        {t(($) => $.settings.identitySecret.description)}
      </Text>
      {secret ? (
        <Group gap="sm" mb="sm">
          <Code>{secret}</Code>
          <CopyTextButton value={secret} size="xs" />
        </Group>
      ) : (
        <Text size="sm" mb="sm">
          {t(($) => $.settings.identitySecret.none)}
        </Text>
      )}
      <Code block>{IDENTIFY_SNIPPET}</Code>
      <Button
        size="xs"
        mt="sm"
        variant={secret ? 'default' : 'filled'}
        loading={regenerate.isPending}
        disabled={query.isPending}
        onClick={onGenerate}
      >
        {secret
          ? t(($) => $.settings.identitySecret.regenerate)
          : t(($) => $.settings.identitySecret.generate)}
      </Button>
    </Box>
  )
}
