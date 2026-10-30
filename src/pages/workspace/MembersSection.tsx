import {
  Box,
  EmptyState,
  Alert,
  Badge,
  Button,
  Code,
  CopyButton,
  Group,
  Loader,
  Menu,
  Modal,
  Select,
  Stack,
  Tabs,
  TextInput,
  Title,
} from '@mantine/core'
import { useForm } from '@mantine/form'
import { useDisclosure } from '@mantine/hooks'
import { useQuery } from '@tanstack/react-query'
import { DataTable } from 'mantine-datatable'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { RowActionsMenu } from '../../components/RowActionsMenu.tsx'
import { TableSkeleton } from '../../components/Skeletons.tsx'
import {
  siteInvitationsCreateMutation,
  siteInvitationsDeleteMutation,
  siteInvitationsListOptions,
  siteInvitationsListQueryKey,
  siteMembershipsDeleteMutation,
  siteMembershipsListOptions,
  siteMembershipsListQueryKey,
  siteMembershipsResetSecondFactorMutation,
  siteMembershipsUpdateMutation,
  siteUserGetMeOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteInvitableRole, SiteWorkspaceResource } from '../../generated/site/types.gen.ts'
import { zSiteInvitableRole, zSiteMembershipRole } from '../../generated/site/zod.gen.ts'
import { useConfirmation } from '../../hooks/useConfirmation.tsx'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { formatDate } from '../../utils/datetime.ts'
import { can } from '../../utils/permissions.ts'

const MEMBER_ROLES = zSiteMembershipRole.options
const INVITABLE_ROLES = zSiteInvitableRole.options

// MembersSection lists the members and pending invitations to everyone; inviting, revoking an
// invitation, changing a Role and removing a member are offered only with members:manage, and
// resetting a Second factor only with second_factor:manage (the API refuses everyone else).
export function MembersSection({ workspace }: { workspace: SiteWorkspaceResource }) {
  const workspaceId = workspace.id
  const { t } = useTranslation()
  const confirmRemove = useConfirmation()
  const [inviteUrl, setInviteUrl] = useState<string | null>(null)
  const [inviteOpened, { open: openInvite, close }] = useDisclosure(false)
  const closeInvite = () => {
    close()
    setInviteUrl(null)
  }
  const me = useQuery(siteUserGetMeOptions())
  const canManage = can(workspace, 'members:manage')
  const canResetSecondFactor = can(workspace, 'second_factor:manage')

  const membersKey = siteMembershipsListQueryKey({ path: { workspaceId } })
  const invitesKey = siteInvitationsListQueryKey({ path: { workspaceId } })
  const membersQuery = useQuery(siteMembershipsListOptions({ path: { workspaceId } }))
  const invitesQuery = useQuery(siteInvitationsListOptions({ path: { workspaceId } }))

  const form = useForm<{ email: string; role: SiteInvitableRole }>({
    initialValues: { email: '', role: 'member' },
  })

  const inviteMutation = useResourceMutation({
    mutation: siteInvitationsCreateMutation(),
    invalidate: [invitesKey],
    errorTitle: t(($) => $.settings.members.inviteError),
    onDone: (data) => {
      setInviteUrl(data.inviteUrl)
      form.reset()
    },
  })

  const revokeMutation = useResourceMutation({
    mutation: siteInvitationsDeleteMutation(),
    invalidate: [invitesKey],
    successMessage: t(($) => $.settings.members.revoked),
    errorTitle: t(($) => $.settings.members.revokeError),
  })

  const roleMutation = useResourceMutation({
    mutation: siteMembershipsUpdateMutation(),
    invalidate: [membersKey],
    successMessage: t(($) => $.settings.members.roleUpdated),
    errorTitle: t(($) => $.settings.members.roleUpdateError),
  })

  const removeMutation = useResourceMutation({
    mutation: siteMembershipsDeleteMutation(),
    invalidate: [membersKey],
    successMessage: t(($) => $.settings.members.removed),
    errorTitle: t(($) => $.settings.members.removeError),
  })

  const resetMutation = useResourceMutation({
    mutation: siteMembershipsResetSecondFactorMutation(),
    invalidate: [membersKey],
    successMessage: t(($) => $.settings.members.secondFactorReset),
    errorTitle: t(($) => $.settings.members.resetSecondFactorError),
  })

  const onResetSecondFactor = (id: string) =>
    confirmRemove({
      title: t(($) => $.settings.members.resetSecondFactorConfirmTitle),
      description: t(($) => $.settings.members.resetSecondFactorConfirmDescription),
      confirmLabel: t(($) => $.settings.members.resetSecondFactorConfirm),
      onConfirm: () => resetMutation.mutate({ path: { workspaceId, id } }),
    })

  const onRemove = (id: string) =>
    confirmRemove({
      title: t(($) => $.settings.members.removeConfirmTitle),
      description: t(($) => $.settings.members.removeConfirmDescription),
      onConfirm: () => removeMutation.mutate({ path: { workspaceId, id } }),
    })

  const onRevoke = (id: string) =>
    confirmRemove({
      title: t(($) => $.settings.members.revokeConfirmTitle),
      description: t(($) => $.settings.members.revokeConfirmDescription),
      onConfirm: () => revokeMutation.mutate({ path: { workspaceId, id } }),
    })

  return (
    <Box>
      <Group justify="space-between" mb="sm">
        <Title order={4}>{t(($) => $.settings.members.title)}</Title>
        {canManage ? (
          <Button size="xs" onClick={openInvite}>
            {t(($) => $.settings.members.invite)}
          </Button>
        ) : null}
      </Group>

      {membersQuery.isError ? (
        <Alert color="red" title={t(($) => $.settings.members.loadError)} mb="md">
          {t(($) => $.settings.members.loadError)}
        </Alert>
      ) : null}

      {membersQuery.isLoading ? <TableSkeleton /> : null}

      <Tabs defaultValue="members" keepMounted={false}>
        <Tabs.List mb="md">
          <Tabs.Tab value="members">{t(($) => $.settings.members.membersTitle)}</Tabs.Tab>
          <Tabs.Tab
            value="invitations"
            rightSection={
              invitesQuery.data?.length ? <Badge size="sm">{invitesQuery.data.length}</Badge> : null
            }
          >
            {t(($) => $.settings.members.invitationsTitle)}
          </Tabs.Tab>
        </Tabs.List>

        <Tabs.Panel value="members">
          <DataTable
            minHeight={membersQuery.data?.length ? 0 : 200}
            withTableBorder
            records={membersQuery.data ?? []}
            idAccessor="id"
            columns={[
              { accessor: 'name', title: t(($) => $.settings.members.nameHeader) },
              { accessor: 'email', title: t(($) => $.settings.members.emailLabel) },
              {
                accessor: 'role',
                title: t(($) => $.settings.members.roleLabel),
                render: (record) => {
                  if (!canManage) {
                    return record.role
                  }
                  // While the change is saved the row shows the chosen Role, locked, with a loader.
                  const changing =
                    roleMutation.isPending && roleMutation.variables.path.id === record.id
                  return (
                    <Select
                      size="xs"
                      w={130}
                      allowDeselect={false}
                      data={MEMBER_ROLES}
                      value={changing ? roleMutation.variables.body.role : record.role}
                      disabled={changing}
                      rightSection={changing ? <Loader size={14} /> : undefined}
                      onChange={(value) => {
                        if (value && value !== record.role) {
                          roleMutation.mutate({
                            path: { workspaceId, id: record.id },
                            body: { role: value },
                          })
                        }
                      }}
                    />
                  )
                },
              },
              {
                accessor: 'source',
                title: t(($) => $.settings.members.sourceLabel),
                render: ({ source }) =>
                  source ? t(($) => $.settings.members.source[source]) : null,
              },
              {
                accessor: 'actions',
                title: '',
                render: (record) => {
                  const canReset = canResetSecondFactor && record.userId !== me.data?.id
                  return canManage || canReset ? (
                    <Group justify="flex-end">
                      <RowActionsMenu>
                        {canReset ? (
                          <Menu.Item color="orange" onClick={() => onResetSecondFactor(record.id)}>
                            {t(($) => $.settings.members.resetSecondFactor)}
                          </Menu.Item>
                        ) : null}
                        {canManage ? (
                          <Menu.Item color="red" onClick={() => onRemove(record.id)}>
                            {t(($) => $.settings.members.remove)}
                          </Menu.Item>
                        ) : null}
                      </RowActionsMenu>
                    </Group>
                  ) : null
                },
              },
            ]}
            emptyState={<EmptyState title={t(($) => $.settings.members.membersEmpty)} />}
          />
        </Tabs.Panel>

        <Tabs.Panel value="invitations">
          <DataTable
            minHeight={invitesQuery.data?.length ? 0 : 200}
            withTableBorder
            records={invitesQuery.data ?? []}
            idAccessor="id"
            columns={[
              { accessor: 'email', title: t(($) => $.settings.members.emailLabel) },
              { accessor: 'role', title: t(($) => $.settings.members.roleLabel) },
              {
                accessor: 'expiresAt',
                title: t(($) => $.settings.members.expires),
                render: (record) => formatDate(record.expiresAt),
              },
              {
                accessor: 'actions',
                title: '',
                render: (record) =>
                  canManage ? (
                    <Group justify="flex-end">
                      <RowActionsMenu>
                        <Menu.Item color="red" onClick={() => onRevoke(record.id)}>
                          {t(($) => $.settings.members.revoke)}
                        </Menu.Item>
                      </RowActionsMenu>
                    </Group>
                  ) : null,
              },
            ]}
            emptyState={<EmptyState title={t(($) => $.settings.members.invitationsEmpty)} />}
          />
        </Tabs.Panel>
      </Tabs>

      <Modal
        opened={inviteOpened}
        onClose={closeInvite}
        title={t(($) => $.settings.members.invite)}
      >
        {inviteUrl ? (
          <Alert color="yellow" title={t(($) => $.settings.members.inviteUrlNotice)}>
            <Group align="center" gap="sm" wrap="nowrap">
              <Code>{inviteUrl}</Code>
              <CopyButton value={inviteUrl}>
                {({ copied, copy }) => (
                  <Button size="compact-sm" variant="light" onClick={copy}>
                    {copied ? t(($) => $.settings.copied) : t(($) => $.settings.copy)}
                  </Button>
                )}
              </CopyButton>
            </Group>
          </Alert>
        ) : (
          <form
            onSubmit={form.onSubmit((values) =>
              inviteMutation.mutate({
                path: { workspaceId },
                body: { email: values.email.trim(), role: values.role },
              }),
            )}
          >
            <Stack>
              <TextInput
                label={t(($) => $.settings.members.emailLabel)}
                type="email"
                required
                data-autofocus
                {...form.getInputProps('email')}
              />
              <Select
                label={t(($) => $.settings.members.roleLabel)}
                allowDeselect={false}
                data={INVITABLE_ROLES}
                {...form.getInputProps('role')}
              />
              <Button type="submit" loading={inviteMutation.isPending}>
                {t(($) => $.settings.members.invite)}
              </Button>
            </Stack>
          </form>
        )}
      </Modal>
    </Box>
  )
}
