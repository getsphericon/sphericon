import {
  Badge,
  Box,
  Button,
  Card,
  Code,
  CopyButton,
  Group,
  Modal,
  Stack,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import { useForm } from '@mantine/form'
import { useDisclosure } from '@mantine/hooks'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { QueryBoundary } from '../../../components/QueryBoundary.tsx'
import { TableSkeleton } from '../../../components/Skeletons.tsx'
import {
  siteTrackingDomainsListOptions,
  siteTrackingDomainsRemoveMutation,
  siteTrackingDomainsSetMutation,
  siteTrackingDomainsVerifyMutation,
} from '../../../generated/site/@tanstack/react-query.gen.ts'
import type {
  SiteTrackingDomainResource,
  SiteTrackingState,
  SiteWorkspaceResource,
} from '../../../generated/site/types.gen.ts'
import { useConfirmation } from '../../../hooks/useConfirmation.tsx'
import { useResourceMutation } from '../../../hooks/useResourceMutation.ts'
import { getFieldError } from '../../../utils/apiErrors.ts'
import { can } from '../../../utils/permissions.ts'

const stateColor: Record<SiteTrackingState, string> = {
  pending: 'gray',
  dns_verified: 'yellow',
  serving: 'teal',
}

// TrackingState shows where a host's verification stands and, when something holds it back, why.
function TrackingState({ domain }: { domain: SiteTrackingDomainResource }) {
  const { t } = useTranslation()
  const labels: Record<SiteTrackingState, string> = {
    pending: t(($) => $.settings.trackingDomain.state.pending),
    dns_verified: t(($) => $.settings.trackingDomain.state.dnsVerified),
    serving: t(($) => $.settings.trackingDomain.state.serving),
  }
  return (
    <Group gap="sm">
      <Text fw={600}>{domain.host}</Text>
      <Badge color={stateColor[domain.state]} variant="light">
        {labels[domain.state]}
      </Badge>
    </Group>
  )
}

function HostTakenNote({ domain }: { domain: SiteTrackingDomainResource }) {
  const { t } = useTranslation()
  return domain.reason === 'host_taken' ? (
    <Text c="red" size="xs">
      {t(($) => $.settings.trackingDomain.hostTaken)}
    </Text>
  ) : null
}

function CopyValue({ value }: { value: string }) {
  const { t } = useTranslation()
  return (
    <Group gap="xs" wrap="nowrap" align="center">
      <Code>{value}</Code>
      <CopyButton value={value}>
        {({ copied, copy }) => (
          <Button size="compact-xs" variant="light" aria-label={value} onClick={copy}>
            {copied ? t(($) => $.settings.copied) : t(($) => $.settings.copy)}
          </Button>
        )}
      </CopyButton>
    </Group>
  )
}

// CnameRecord is the record to publish at the DNS provider of the host.
function CnameRecord({ domain }: { domain: SiteTrackingDomainResource }) {
  const { t } = useTranslation()
  return (
    <Stack gap={4}>
      <Text size="sm">
        {t(($) => $.settings.trackingDomain.recordHint, { type: domain.record.type })}
      </Text>
      <Group gap="lg" wrap="wrap">
        <Box>
          <Text size="xs" c="dimmed">
            {t(($) => $.settings.trackingDomain.recordHost)}
          </Text>
          <CopyValue value={domain.record.host} />
        </Box>
        <Box>
          <Text size="xs" c="dimmed">
            {t(($) => $.settings.trackingDomain.recordTarget)}
          </Text>
          <CopyValue value={domain.record.target} />
        </Box>
      </Group>
    </Stack>
  )
}

// HostForm sets a host: the Workspace's first, or another one that replaces the current host. A
// host the API refuses (not a host below a domain, or one of the Instance's own) shows on the field.
function HostForm({
  workspaceId,
  submitLabel,
  onDone,
}: {
  workspaceId: string
  submitLabel: string
  onDone?: () => void
}) {
  const { t } = useTranslation()
  const form = useForm({ initialValues: { host: '' } })
  const setMutation = useResourceMutation({
    mutation: siteTrackingDomainsSetMutation(),
    successMessage: t(($) => $.settings.trackingDomain.setDone),
    errorTitle: t(($) => $.settings.trackingDomain.setError),
    forbiddenMessage: t(($) => $.settings.trackingDomain.forbidden),
    handleError: (error) => {
      const message = getFieldError(error, 'host')
      if (message) {
        form.setFieldError('host', message)
        return true
      }
      return false
    },
    onDone: () => {
      form.reset()
      onDone?.()
    },
  })

  return (
    <form
      onSubmit={form.onSubmit((values) =>
        setMutation.mutate({ path: { workspaceId }, body: { host: values.host.trim() } }),
      )}
    >
      <Group align="flex-end" gap="sm">
        <TextInput
          flex={1}
          miw={220}
          maw={420}
          label={t(($) => $.settings.trackingDomain.hostLabel)}
          description={t(($) => $.settings.trackingDomain.hostDescription)}
          placeholder="t.example.com"
          required
          data-autofocus
          {...form.getInputProps('host')}
        />
        <Button type="submit" loading={setMutation.isPending}>
          {submitLabel}
        </Button>
      </Group>
    </form>
  )
}

// TrackingDomains shows the current host, a pending replacement and the retired hosts, with the
// actions a member with tracking_domains:manage may take; the section's boundary shows the load.
function TrackingDomains({ workspaceId, canManage }: { workspaceId: string; canManage: boolean }) {
  const { t } = useTranslation()
  const confirm = useConfirmation()
  const [changeOpened, { open: openChange, close: closeChange }] = useDisclosure(false)
  const domains = useSuspenseQuery(siteTrackingDomainsListOptions({ path: { workspaceId } })).data
  const current = domains.find((domain) => domain.role === 'current')
  const replacement = domains.find((domain) => domain.role === 'replacement')
  const retired = domains.filter((domain) => domain.role === 'retired')

  const verifyMutation = useResourceMutation({
    mutation: siteTrackingDomainsVerifyMutation(),
    successMessage: t(($) => $.settings.trackingDomain.verifyDone),
    errorTitle: t(($) => $.settings.trackingDomain.verifyError),
    forbiddenMessage: t(($) => $.settings.trackingDomain.forbidden),
  })
  const removeMutation = useResourceMutation({
    mutation: siteTrackingDomainsRemoveMutation(),
    errorTitle: t(($) => $.settings.trackingDomain.removeError),
    forbiddenMessage: t(($) => $.settings.trackingDomain.forbidden),
  })
  const verifyingId = verifyMutation.isPending ? verifyMutation.variables.path.id : null
  const remove = (id: string) => removeMutation.mutate({ path: { workspaceId, id } })
  const verifyButton = (domain: SiteTrackingDomainResource) => (
    <Button
      size="xs"
      loading={verifyingId === domain.id}
      onClick={() => verifyMutation.mutate({ path: { workspaceId, id: domain.id } })}
    >
      {t(($) => $.settings.trackingDomain.verify)}
    </Button>
  )

  // A current host that served (and is still verified) is retired, so the links sent on it keep
  // working; one that never served is deleted. A pending replacement takes its place either way.
  const onRemoveCurrent = (domain: SiteTrackingDomainResource) => {
    const retires = Boolean(domain.servedAt) && domain.state !== 'pending'
    const outcome = retires
      ? t(($) => $.settings.trackingDomain.removeRetires, {
          host: domain.host,
          target: domain.record.target,
        })
      : t(($) => $.settings.trackingDomain.removeDeletes, { host: domain.host })
    const promotes = replacement
      ? ` ${t(($) => $.settings.trackingDomain.removePromotes, { host: replacement.host })}`
      : ''
    confirm({
      title: t(($) => $.settings.trackingDomain.removeTitle),
      description: outcome + promotes,
      confirmLabel: t(($) => $.settings.trackingDomain.remove),
      onConfirm: () => remove(domain.id),
    })
  }
  const onForget = (domain: SiteTrackingDomainResource) => {
    confirm({
      title: t(($) => $.settings.trackingDomain.forgetTitle),
      description: t(($) => $.settings.trackingDomain.forgetDescription, { host: domain.host }),
      confirmLabel: t(($) => $.settings.trackingDomain.forget),
      onConfirm: () => remove(domain.id),
    })
  }

  return (
    <Stack gap="md">
      {current ? (
        <Card withBorder padding="sm">
          <Stack gap="xs">
            <Group justify="space-between">
              <TrackingState domain={current} />
              {canManage ? (
                <Group gap="xs">
                  {verifyButton(current)}
                  <Button size="xs" variant="default" onClick={openChange}>
                    {t(($) => $.settings.trackingDomain.change)}
                  </Button>
                  <Button
                    size="xs"
                    variant="light"
                    color="red"
                    onClick={() => onRemoveCurrent(current)}
                  >
                    {t(($) => $.settings.trackingDomain.remove)}
                  </Button>
                </Group>
              ) : null}
            </Group>
            <HostTakenNote domain={current} />
            <CnameRecord domain={current} />
          </Stack>
        </Card>
      ) : canManage ? (
        <HostForm workspaceId={workspaceId} submitLabel={t(($) => $.settings.trackingDomain.set)} />
      ) : (
        <Text size="sm">{t(($) => $.settings.trackingDomain.empty)}</Text>
      )}

      {replacement ? (
        <Card withBorder padding="sm">
          <Stack gap="xs">
            <Text size="xs" c="dimmed" tt="uppercase" fw={600}>
              {t(($) => $.settings.trackingDomain.replacementTitle)}
            </Text>
            <Group justify="space-between">
              <TrackingState domain={replacement} />
              {canManage ? (
                <Group gap="xs">
                  {verifyButton(replacement)}
                  <Button
                    size="xs"
                    variant="light"
                    color="red"
                    loading={removeMutation.isPending}
                    onClick={() => remove(replacement.id)}
                  >
                    {t(($) => $.settings.trackingDomain.cancelReplacement)}
                  </Button>
                </Group>
              ) : null}
            </Group>
            {current ? (
              <Text size="sm" c="dimmed">
                {t(($) => $.settings.trackingDomain.replacementNote, {
                  current: current.host,
                  host: replacement.host,
                })}
              </Text>
            ) : null}
            <HostTakenNote domain={replacement} />
            <CnameRecord domain={replacement} />
          </Stack>
        </Card>
      ) : null}

      {retired.length > 0 ? (
        <Stack gap="xs">
          <Text size="xs" c="dimmed" tt="uppercase" fw={600}>
            {t(($) => $.settings.trackingDomain.retiredTitle)}
          </Text>
          <Text size="sm" c="dimmed">
            {t(($) => $.settings.trackingDomain.retiredDescription)}
          </Text>
          {retired.map((domain) => (
            <Group key={domain.id} justify="space-between">
              <Text>{domain.host}</Text>
              {canManage ? (
                <Button size="xs" variant="subtle" color="red" onClick={() => onForget(domain)}>
                  {t(($) => $.settings.trackingDomain.forget)}
                </Button>
              ) : null}
            </Group>
          ))}
        </Stack>
      ) : null}

      <Modal
        opened={changeOpened}
        onClose={closeChange}
        title={t(($) => $.settings.trackingDomain.changeTitle)}
      >
        <Stack>
          <Text size="sm">
            {t(($) => $.settings.trackingDomain.changeDescription, { host: current?.host ?? '' })}
          </Text>
          <HostForm
            workspaceId={workspaceId}
            submitLabel={t(($) => $.settings.trackingDomain.set)}
            onDone={closeChange}
          />
        </Stack>
      </Modal>
    </Stack>
  )
}

// TrackingDomainSection lets the Workspace serve its links and its tracker on a host of its own
// (ADR 0071). It shows only when the Instance offers Tracking domains (a CNAME target to point at);
// any member sees the hosts, setting, checking and removing one needs tracking_domains:manage.
export function TrackingDomainSection({ workspace }: { workspace: SiteWorkspaceResource }) {
  const { t } = useTranslation()
  if (!workspace.trackingTarget) {
    return null
  }
  return (
    <Box>
      <Title order={4} mb="xs">
        {t(($) => $.settings.trackingDomain.title)}
      </Title>
      <Text c="dimmed" size="sm" mb="sm">
        {t(($) => $.settings.trackingDomain.description, { target: workspace.trackingTarget })}
      </Text>
      <QueryBoundary
        fallback={<TableSkeleton />}
        errorTitle={t(($) => $.settings.trackingDomain.loadError)}
      >
        <TrackingDomains
          workspaceId={workspace.id}
          canManage={can(workspace, 'tracking_domains:manage')}
        />
      </QueryBoundary>
    </Box>
  )
}
