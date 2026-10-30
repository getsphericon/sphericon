import {
  Alert,
  Badge,
  Button,
  Card,
  Group,
  Stack,
  Switch,
  Table,
  Tabs,
  Text,
  TextInput,
  Textarea,
  Title,
} from '@mantine/core'
import { useForm } from '@mantine/form'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../components/ApiErrorAlert.tsx'
import { LegalMarkdown } from '../components/LegalMarkdown.tsx'
import { AnchorLink } from '../components/RouterLink.tsx'
import { PageSkeleton } from '../components/Skeletons.tsx'
import {
  staffSignupItemsGetOptions,
  staffSignupItemsGetQueryKey,
  staffSignupItemsListQueryKey,
  staffSignupItemsListVersionsOptions,
  staffSignupItemsListVersionsQueryKey,
  staffSignupItemsPublishMutation,
  staffSignupItemsArchiveMutation,
  staffSignupItemsSaveDraftMutation,
  staffSignupItemsUpdateMutation,
} from '../generated/staff/@tanstack/react-query.gen.ts'
import { SignupItemKind } from '../generated/staff/types.gen.ts'
import type { StaffSignupItemResource } from '../generated/staff/types.gen.ts'
import { useConfirmation } from '../hooks/useConfirmation.tsx'
import { useResourceMutation } from '../hooks/useResourceMutation.ts'
import { Route as consoleSignupItemRoute } from '../routes/console/_authed/signup-items.$itemId.tsx'
import { Route as legalDocumentRoute } from '../routes/legal.$key.tsx'
import { formatDateTime } from '../utils/datetime.ts'
import { publishStopsUsers } from '../utils/signupItems.ts'

function useInvalidate(itemId: string) {
  return [staffSignupItemsGetQueryKey({ path: { id: itemId } }), staffSignupItemsListQueryKey()]
}

// The title the sign-up form shows: changing it is not a new version of the document.
function TitleForm({ item }: { item: StaffSignupItemResource }) {
  const { t } = useTranslation()
  const form = useForm({
    initialValues: { title: item.title },
    validate: {
      title: (value) => (value.trim() ? null : t(($) => $.console.signupItem.titleRequired)),
    },
  })
  const mutation = useResourceMutation({
    mutation: staffSignupItemsUpdateMutation(),
    invalidate: useInvalidate(item.id),
    successMessage: t(($) => $.console.signupItem.titleSaved),
    errorTitle: t(($) => $.console.signupItem.titleError),
  })

  return (
    <form
      onSubmit={form.onSubmit(({ title }) =>
        mutation.mutate({ path: { id: item.id }, body: { title: title.trim() } }),
      )}
    >
      <Group align="flex-end">
        <TextInput
          label={t(($) => $.console.signupItem.titleLabel)}
          description={t(($) => $.console.signupItem.titleDescription)}
          miw={260}
          {...form.getInputProps('title')}
        />
        <Button type="submit" variant="default" loading={mutation.isPending}>
          {t(($) => $.console.signupItem.saveTitle)}
        </Button>
      </Group>
    </form>
  )
}

// Archive the item: it leaves the sign-up form and its public page, and cannot be put back.
function ArchiveButton({ item }: { item: StaffSignupItemResource }) {
  const { t } = useTranslation()
  const confirm = useConfirmation()
  const mutation = useResourceMutation({
    mutation: staffSignupItemsArchiveMutation(),
    invalidate: useInvalidate(item.id),
    successMessage: t(($) => $.console.signupItem.archiveDone),
    errorTitle: t(($) => $.console.signupItem.archiveError),
  })

  return (
    <Button
      color="red"
      variant="light"
      loading={mutation.isPending}
      onClick={() =>
        confirm({
          title: t(($) => $.console.signupItem.archiveTitle),
          description: t(($) => $.console.signupItem.archiveDescription),
          confirmLabel: t(($) => $.console.signupItem.archive),
          onConfirm: () => mutation.mutate({ path: { id: item.id } }),
        })
      }
    >
      {t(($) => $.console.signupItem.archive)}
    </Button>
  )
}

// Every published version with who published it and when, the current one first.
function VersionHistory({ itemId }: { itemId: string }) {
  const { t } = useTranslation()
  const versionsQuery = useQuery(staffSignupItemsListVersionsOptions({ path: { id: itemId } }))

  if (versionsQuery.isPending) {
    return <PageSkeleton />
  }
  if (versionsQuery.isError) {
    return (
      <ApiErrorAlert
        error={versionsQuery.error}
        title={t(($) => $.console.signupItem.versionsLoadError)}
        fallback={t(($) => $.console.signupItem.versionsLoadError)}
      />
    )
  }
  const versions = versionsQuery.data.items
  if (versions.length === 0) {
    return <Text c="dimmed">{t(($) => $.console.signupItem.noVersions)}</Text>
  }

  return (
    <Table.ScrollContainer minWidth={400}>
      <Table>
        <Table.Thead>
          <Table.Tr>
            <Table.Th>{t(($) => $.console.signupItem.versionColumns.label)}</Table.Th>
            <Table.Th>{t(($) => $.console.signupItem.versionColumns.publishedBy)}</Table.Th>
            <Table.Th>{t(($) => $.console.signupItem.versionColumns.publishedAt)}</Table.Th>
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>
          {versions.map((version) => (
            <Table.Tr key={version.label}>
              <Table.Td>{version.label}</Table.Td>
              <Table.Td>{version.publishedBy.email}</Table.Td>
              <Table.Td>{formatDateTime(version.publishedAt)}</Table.Td>
            </Table.Tr>
          ))}
        </Table.Tbody>
      </Table>
    </Table.ScrollContainer>
  )
}

// Whether the optional consent stands for consent to the operator's mailings: one item at most,
// which the server enforces.
function MailingsSwitch({ item }: { item: StaffSignupItemResource }) {
  const { t } = useTranslation()
  const mutation = useResourceMutation({
    mutation: staffSignupItemsUpdateMutation(),
    invalidate: useInvalidate(item.id),
    errorTitle: t(($) => $.console.signupItem.mailingsError),
  })

  return (
    <Stack gap={4}>
      <Switch
        label={t(($) => $.console.signupItems.mailingsLabel)}
        checked={item.mailingsConsent}
        disabled={mutation.isPending}
        onChange={(event) =>
          mutation.mutate({
            path: { id: item.id },
            body: { mailingsConsent: event.currentTarget.checked },
          })
        }
      />
      <Text size="xs" c="dimmed">
        {t(($) => $.console.signupItems.mailingsDescription)}
      </Text>
    </Stack>
  )
}

// The draft of the next version: Markdown written and saved freely, seen by no visitor until it
// is published. Publishing takes the saved draft, so it waits while the text has unsaved edits.
function DraftEditor({ item }: { item: StaffSignupItemResource }) {
  const { t } = useTranslation()
  const form = useForm({ initialValues: { markdown: item.draft ?? '' } })
  const mutation = useResourceMutation({
    mutation: staffSignupItemsSaveDraftMutation(),
    invalidate: useInvalidate(item.id),
    successMessage: t(($) => $.console.signupItem.draftSaved),
    errorTitle: t(($) => $.console.signupItem.draftError),
  })

  return (
    <Stack>
      <form
        onSubmit={form.onSubmit(({ markdown }) =>
          mutation.mutate({ path: { id: item.id }, body: { markdown } }),
        )}
      >
        <Stack>
          <Tabs defaultValue="write">
            <Tabs.List>
              <Tabs.Tab value="write">{t(($) => $.console.signupItem.write)}</Tabs.Tab>
              <Tabs.Tab value="preview">{t(($) => $.console.signupItem.preview)}</Tabs.Tab>
            </Tabs.List>
            <Tabs.Panel value="write" pt="sm">
              <Textarea
                label={t(($) => $.console.signupItem.draftLabel)}
                description={t(($) => $.console.signupItem.draftDescription)}
                autosize
                minRows={12}
                {...form.getInputProps('markdown')}
              />
            </Tabs.Panel>
            <Tabs.Panel value="preview" pt="sm">
              <Card withBorder padding="lg">
                <LegalMarkdown markdown={form.values.markdown} />
              </Card>
            </Tabs.Panel>
          </Tabs>
          <Group justify="flex-end">
            <Button type="submit" variant="default" loading={mutation.isPending}>
              {t(($) => $.console.signupItem.saveDraft)}
            </Button>
          </Group>
        </Stack>
      </form>
      <PublishForm item={item} unsaved={form.isDirty()} />
    </Stack>
  )
}

// Publish the saved draft under a label: an immutable version that takes effect at once. A new
// version of a required acceptance in effect stops every User who has not accepted it (ADR 0062),
// so it is published only once the Staff member confirms the warning.
function PublishForm({ item, unsaved }: { item: StaffSignupItemResource; unsaved: boolean }) {
  const { t } = useTranslation()
  const confirm = useConfirmation()
  const form = useForm({
    initialValues: { label: '' },
    validate: {
      label: (value) => (value.trim() ? null : t(($) => $.console.signupItem.labelRequired)),
    },
  })
  const mutation = useResourceMutation({
    mutation: staffSignupItemsPublishMutation(),
    invalidate: [
      ...useInvalidate(item.id),
      staffSignupItemsListVersionsQueryKey({ path: { id: item.id } }),
    ],
    successMessage: t(($) => $.console.signupItem.published),
    errorTitle: t(($) => $.console.signupItem.publishError),
    onDone: () => form.reset(),
  })
  const publish = (label: string) =>
    mutation.mutate({ path: { id: item.id }, body: { label: label.trim() } })

  return (
    <form
      onSubmit={form.onSubmit(({ label }) => {
        if (!publishStopsUsers(item)) {
          publish(label)
          return
        }
        confirm({
          title: t(($) => $.console.signupItem.publishWarningTitle),
          description: t(($) => $.console.signupItem.publishWarning),
          confirmLabel: t(($) => $.console.signupItem.publish),
          onConfirm: () => publish(label),
        })
      })}
    >
      <Stack gap="xs">
        <Group align="flex-end">
          <TextInput
            label={t(($) => $.console.signupItem.labelLabel)}
            description={t(($) => $.console.signupItem.labelDescription)}
            {...form.getInputProps('label')}
          />
          <Button
            type="submit"
            loading={mutation.isPending}
            disabled={item.draft === undefined || unsaved}
          >
            {t(($) => $.console.signupItem.publish)}
          </Button>
        </Group>
        {unsaved ? (
          <Text size="xs" c="dimmed">
            {t(($) => $.console.signupItem.saveBeforePublishing)}
          </Text>
        ) : null}
      </Stack>
    </form>
  )
}

// A Sign-up item and its Legal document (ADR 0062): the draft, its publishing, and the version in
// effect with a link to its public page.
export function ConsoleSignupItemPage() {
  const { t } = useTranslation()
  const { itemId } = consoleSignupItemRoute.useParams()
  const itemQuery = useQuery(staffSignupItemsGetOptions({ path: { id: itemId } }))

  if (itemQuery.isPending) {
    return <PageSkeleton />
  }
  if (itemQuery.isError) {
    return (
      <ApiErrorAlert
        error={itemQuery.error}
        title={t(($) => $.console.signupItem.loadError)}
        fallback={t(($) => $.console.signupItem.loadError)}
      />
    )
  }
  const item = itemQuery.data

  return (
    <Stack>
      <Group justify="space-between" align="flex-start">
        <Stack gap={4}>
          <Title order={2}>{item.title}</Title>
          <Group gap="xs">
            <Badge variant="light">{t(($) => $.console.signupItems.kind[item.kind])}</Badge>
            {item.archivedAt ? (
              <Badge color="gray" variant="outline">
                {t(($) => $.console.signupItem.archived)}
              </Badge>
            ) : null}
            <Text size="sm" c="dimmed">
              {item.key}
            </Text>
          </Group>
        </Stack>
        {item.archivedAt ? null : <ArchiveButton item={item} />}
      </Group>
      {item.archivedAt ? (
        <Alert color="gray" variant="light">
          {t(($) => $.console.signupItem.archivedDescription, {
            date: formatDateTime(item.archivedAt),
          })}
        </Alert>
      ) : null}
      {/* Remounted when the saved title changes, so the form starts from it. */}
      <TitleForm key={item.title} item={item} />
      {item.kind === SignupItemKind.OPTIONAL_CONSENT ? <MailingsSwitch item={item} /> : null}
      <Card withBorder>
        <Stack gap="xs">
          {item.currentVersion ? (
            <>
              <Text>
                {t(($) => $.console.signupItem.currentVersion, {
                  label: item.currentVersion.label,
                  date: formatDateTime(item.currentVersion.publishedAt),
                })}
              </Text>
              <AnchorLink to={legalDocumentRoute.to} params={{ key: item.key }} target="_blank">
                {t(($) => $.console.signupItem.publicPage)}
              </AnchorLink>
            </>
          ) : (
            <Text c="dimmed">{t(($) => $.console.signupItem.noVersion)}</Text>
          )}
        </Stack>
      </Card>
      <Card withBorder>
        <Stack>
          <Title order={4}>{t(($) => $.console.signupItem.draftTitle)}</Title>
          {/* Remounted when the saved draft changes, so the editor starts from it. */}
          <DraftEditor key={item.draft ?? ''} item={item} />
        </Stack>
      </Card>
      <Card withBorder>
        <Stack>
          <Title order={4}>{t(($) => $.console.signupItem.versionsTitle)}</Title>
          <VersionHistory itemId={item.id} />
        </Stack>
      </Card>
    </Stack>
  )
}
