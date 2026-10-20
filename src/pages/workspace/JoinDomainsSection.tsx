import {
  Alert,
  Badge,
  Box,
  Button,
  Card,
  Code,
  CopyButton,
  Group,
  Loader,
  Stack,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import { useForm } from '@mantine/form'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import {
  siteJoinDomainsAddMutation,
  siteJoinDomainsListOptions,
  siteJoinDomainsListQueryKey,
  siteJoinDomainsRemoveMutation,
  siteJoinDomainsVerifyMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteJoinDomainResource } from '../../generated/site/types.gen.ts'
import { useDeleteConfirmation } from '../../hooks/useDeleteConfirmation.tsx'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'

interface DomainFormValues {
  domain: string
}

// JoinDomainRow shows one Join domain with its state, the TXT record still to publish while it
// is pending, and the Verify and Remove actions.
function JoinDomainRow({
  domain,
  verifying,
  onVerify,
  onRemove,
}: {
  domain: SiteJoinDomainResource
  verifying: boolean
  onVerify: () => void
  onRemove: () => void
}) {
  const { t } = useTranslation()
  return (
    <Card withBorder padding="sm">
      <Stack gap="xs">
        <Group justify="space-between">
          <Group gap="sm">
            <Text fw={600}>{domain.domain}</Text>
            {domain.verified ? (
              <Badge color="teal" variant="light">
                {t(($) => $.joinDomains.verified)}
              </Badge>
            ) : (
              <Badge color="gray" variant="light">
                {t(($) => $.joinDomains.pending)}
              </Badge>
            )}
          </Group>
          <Group gap="xs">
            {domain.verified ? null : (
              <Button size="xs" loading={verifying} onClick={onVerify}>
                {t(($) => $.joinDomains.verify)}
              </Button>
            )}
            <Button size="xs" color="red" variant="light" onClick={onRemove}>
              {t(($) => $.joinDomains.remove)}
            </Button>
          </Group>
        </Group>
        {domain.verified ? null : (
          <Stack gap={4}>
            <Text size="sm">{t(($) => $.joinDomains.recordHint)}</Text>
            <Group gap="xs">
              <Text size="xs" c="dimmed">
                {t(($) => $.joinDomains.recordHost)}
              </Text>
              <Code>{domain.record.host}</Code>
            </Group>
            <Group gap="xs" wrap="nowrap" align="flex-start">
              <Text size="xs" c="dimmed">
                {t(($) => $.joinDomains.recordValue)}
              </Text>
              <Code block maw={420}>
                {domain.record.value}
              </Code>
              <CopyButton value={domain.record.value}>
                {({ copied, copy }) => (
                  <Button size="compact-xs" variant="light" onClick={copy}>
                    {copied ? '✓' : '⧉'}
                  </Button>
                )}
              </CopyButton>
            </Group>
          </Stack>
        )}
      </Stack>
    </Card>
  )
}

// JoinDomainsSection is the Owner's "Domain access" panel: add a domain, publish
// the TXT record it shows, Verify, and remove. Owners only: the page does not render it for
// anyone else, and the API refuses them.
export function JoinDomainsSection({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation()
  const confirmDelete = useDeleteConfirmation()
  const queryKey = siteJoinDomainsListQueryKey({ path: { workspaceId } })
  const domainsQuery = useQuery(siteJoinDomainsListOptions({ path: { workspaceId } }))
  const form = useForm<DomainFormValues>({ initialValues: { domain: '' } })

  const addMutation = useResourceMutation({
    mutation: siteJoinDomainsAddMutation(),
    invalidate: [queryKey],
    successMessage: t(($) => $.joinDomains.added),
    errorTitle: t(($) => $.joinDomains.addError),
    forbiddenMessage: t(($) => $.joinDomains.forbidden),
    onDone: () => form.reset(),
  })
  const verifyMutation = useResourceMutation({
    mutation: siteJoinDomainsVerifyMutation(),
    invalidate: [queryKey],
    successMessage: t(($) => $.joinDomains.verifiedToast),
    errorTitle: t(($) => $.joinDomains.verifyError),
    forbiddenMessage: t(($) => $.joinDomains.forbidden),
  })
  const removeMutation = useResourceMutation({
    mutation: siteJoinDomainsRemoveMutation(),
    invalidate: [queryKey],
    errorTitle: t(($) => $.joinDomains.removeError),
    forbiddenMessage: t(($) => $.joinDomains.forbidden),
  })

  const onRemove = (id: string) => {
    confirmDelete({
      title: t(($) => $.joinDomains.removeConfirmTitle),
      description: t(($) => $.joinDomains.removeConfirmDescription),
      confirmLabel: t(($) => $.joinDomains.remove),
      onConfirm: () => removeMutation.mutate({ path: { workspaceId, id } }),
    })
  }

  const domains = domainsQuery.data ?? []

  return (
    <Box>
      <Title order={4} mb="xs">
        {t(($) => $.joinDomains.title)}
      </Title>
      <Stack gap="sm">
        <Text c="dimmed" size="sm">
          {t(($) => $.joinDomains.description)}
        </Text>

        {domainsQuery.isError ? (
          <Alert color="red" title={t(($) => $.joinDomains.loadError)} />
        ) : null}
        {domainsQuery.isLoading ? <Loader /> : null}
        {domainsQuery.isSuccess && domains.length === 0 ? (
          <Text size="sm">{t(($) => $.joinDomains.empty)}</Text>
        ) : null}

        {domains.map((domain) => (
          <JoinDomainRow
            key={domain.id}
            domain={domain}
            verifying={verifyMutation.isPending && verifyMutation.variables.path.id === domain.id}
            onVerify={() => verifyMutation.mutate({ path: { workspaceId, id: domain.id } })}
            onRemove={() => onRemove(domain.id)}
          />
        ))}

        <form
          onSubmit={form.onSubmit((values) =>
            addMutation.mutate({ path: { workspaceId }, body: { domain: values.domain.trim() } }),
          )}
        >
          <Group align="flex-end">
            <TextInput
              label={t(($) => $.joinDomains.domainLabel)}
              placeholder="acme.com"
              required
              {...form.getInputProps('domain')}
            />
            <Button type="submit" loading={addMutation.isPending}>
              {t(($) => $.joinDomains.add)}
            </Button>
          </Group>
        </form>
      </Stack>
    </Box>
  )
}
