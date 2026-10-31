import {
  Badge,
  Box,
  Button,
  Card,
  Code,
  CopyButton,
  Group,
  Menu,
  Modal,
  Stack,
  Switch,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import { useForm } from '@mantine/form'
import { useDisclosure } from '@mantine/hooks'
import { useQuery, useSuspenseQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { RowActionsMenu } from '../../components/RowActionsMenu.tsx'
import { TableSkeleton } from '../../components/Skeletons.tsx'
import {
  siteDomainJoinGetOptions,
  siteDomainJoinGetQueryKey,
  siteDomainJoinSetMutation,
  siteJoinDomainsAddMutation,
  siteJoinDomainsListOptions,
  siteJoinDomainsListQueryKey,
  siteJoinDomainsRemoveMutation,
  siteJoinDomainsVerifyMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteJoinDomainResource } from '../../generated/site/types.gen.ts'
import { useConfirmation } from '../../hooks/useConfirmation.tsx'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'

interface DomainFormValues {
  domain: string
}

// JoinDomainRow shows one Join domain with its state, the TXT record still to publish while it
// is pending, a Verify button and Remove in the row's menu.
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
            <RowActionsMenu>
              <Menu.Item color="red" onClick={onRemove}>
                {t(($) => $.joinDomains.remove)}
              </Menu.Item>
            </RowActionsMenu>
          </Group>
        </Group>
        {domain.verified ? null : (
          <Stack gap={4}>
            <Text size="sm">{t(($) => $.joinDomains.recordHint)}</Text>
            <div>
              <Text size="xs" c="dimmed">
                {t(($) => $.joinDomains.recordHost)}
              </Text>
              <Code>{domain.record.host}</Code>
            </div>
            <div>
              <Text size="xs" c="dimmed">
                {t(($) => $.joinDomains.recordValue)}
              </Text>
              <Group gap="xs" wrap="nowrap" align="center">
                <Code>{domain.record.value}</Code>
                <CopyButton value={domain.record.value}>
                  {({ copied, copy }) => (
                    <Button size="compact-xs" variant="light" onClick={copy}>
                      {copied ? '✓' : '⧉'}
                    </Button>
                  )}
                </CopyButton>
              </Group>
            </div>
          </Stack>
        )}
      </Stack>
    </Card>
  )
}

// JoinDomainsList shows each domain with its record; the section's boundary shows the load and
// its failure.
function JoinDomainsList({
  workspaceId,
  verifyingId,
  onVerify,
  onRemove,
}: {
  workspaceId: string
  verifyingId: string | null
  onVerify: (id: string) => void
  onRemove: (id: string) => void
}) {
  const { t } = useTranslation()
  const domains = useSuspenseQuery(siteJoinDomainsListOptions({ path: { workspaceId } })).data

  if (domains.length === 0) {
    return <Text size="sm">{t(($) => $.joinDomains.empty)}</Text>
  }
  return domains.map((domain) => (
    <JoinDomainRow
      key={domain.id}
      domain={domain}
      verifying={verifyingId === domain.id}
      onVerify={() => onVerify(domain.id)}
      onRemove={() => onRemove(domain.id)}
    />
  ))
}

// JoinDomainsSection is the "Domain access" panel: add a domain, publish the TXT record it
// shows, Verify, remove, and switch domain join on or off (a person who proves an address on a
// verified domain after that becomes a member). It needs join_domains:manage: the page does not
// render it for anyone else, and the API refuses them.
export function JoinDomainsSection({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation()
  const confirmDelete = useConfirmation()
  const queryKey = siteJoinDomainsListQueryKey({ path: { workspaceId } })
  const joinKey = siteDomainJoinGetQueryKey({ path: { workspaceId } })
  const joinQuery = useQuery(siteDomainJoinGetOptions({ path: { workspaceId } }))
  const [addOpened, { open: openAdd, close: closeAdd }] = useDisclosure(false)
  const form = useForm<DomainFormValues>({ initialValues: { domain: '' } })

  const joinMutation = useResourceMutation({
    mutation: siteDomainJoinSetMutation(),
    invalidate: [joinKey],
    successMessage: t(($) => $.joinDomains.switchSaved),
    errorTitle: t(($) => $.joinDomains.switchError),
    forbiddenMessage: t(($) => $.joinDomains.forbidden),
  })
  const addMutation = useResourceMutation({
    mutation: siteJoinDomainsAddMutation(),
    invalidate: [queryKey],
    successMessage: t(($) => $.joinDomains.added),
    errorTitle: t(($) => $.joinDomains.addError),
    forbiddenMessage: t(($) => $.joinDomains.forbidden),
    onDone: () => {
      form.reset()
      closeAdd()
    },
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

  return (
    <Box>
      <Title order={4} mb="xs">
        {t(($) => $.joinDomains.title)}
      </Title>
      <Stack gap="sm">
        <Text c="dimmed" size="sm">
          {t(($) => $.joinDomains.description)}
        </Text>

        <Switch
          label={t(($) => $.joinDomains.switchLabel)}
          description={t(($) => $.joinDomains.switchDescription)}
          checked={joinQuery.data?.enabled ?? false}
          disabled={!joinQuery.isSuccess || joinMutation.isPending}
          onChange={(event) =>
            joinMutation.mutate({
              path: { workspaceId },
              body: { enabled: event.currentTarget.checked },
            })
          }
        />

        <QueryBoundary fallback={<TableSkeleton />} errorTitle={t(($) => $.joinDomains.loadError)}>
          <JoinDomainsList
            workspaceId={workspaceId}
            verifyingId={verifyMutation.isPending ? verifyMutation.variables.path.id : null}
            onVerify={(id) => verifyMutation.mutate({ path: { workspaceId, id } })}
            onRemove={onRemove}
          />
        </QueryBoundary>

        <Group>
          <Button onClick={openAdd}>{t(($) => $.joinDomains.add)}</Button>
        </Group>
      </Stack>

      <Modal opened={addOpened} onClose={closeAdd} title={t(($) => $.joinDomains.add)}>
        <form
          onSubmit={form.onSubmit((values) =>
            addMutation.mutate({ path: { workspaceId }, body: { domain: values.domain.trim() } }),
          )}
        >
          <Stack>
            <TextInput
              label={t(($) => $.joinDomains.domainLabel)}
              placeholder="acme.com"
              required
              data-autofocus
              {...form.getInputProps('domain')}
            />
            <Button type="submit" loading={addMutation.isPending}>
              {t(($) => $.joinDomains.add)}
            </Button>
          </Stack>
        </form>
      </Modal>
    </Box>
  )
}
