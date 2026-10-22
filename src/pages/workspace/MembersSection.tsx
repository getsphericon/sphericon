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
import { useNavigate } from '@tanstack/react-router'
import { DataTable } from 'mantine-datatable'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { RowActionsMenu } from '../../components/RowActionsMenu.tsx'
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
  siteWorkspacesLeaveMutation,
  siteWorkspacesListQueryKey,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteInvitableRole, SiteMembershipRole } from '../../generated/site/types.gen.ts'
import { canManage, useCurrentRole } from '../../hooks/useCurrentRole.ts'
import { useDeleteConfirmation } from '../../hooks/useDeleteConfirmation.tsx'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as indexRoute } from '../../routes/index.tsx'
import { formatDate } from '../../utils/datetime.ts'

const MEMBER_ROLES: SiteMembershipRole[] = ['owner', 'admin', 'member']
const INVITABLE_ROLES: SiteInvitableRole[] = ['admin', 'member']

export function MembersSection({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation()
  const confirmRemove = useDeleteConfirmation()
  const navigate = useNavigate()
  const [inviteUrl, setInviteUrl] = useState<string | null>(null)
  const [inviteOpened, { open: openInvite, close }] = useDisclosure(false)
  const closeInvite = () => {
    close()
    setInviteUrl(null)
  }
  const role = useCurrentRole(workspaceId)
  const me = useQuery(siteUserGetMeOptions())
  const canManageMembers = canManage(role)

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
    errorTitle: t(($) => $.settings.members.revokeError),
  })

  const roleMutation = useResourceMutation({
    mutation: siteMembershipsUpdateMutation(),
    invalidate: [membersKey],
    errorTitle: t(($) => $.settings.members.roleUpdateError),
  })

  const removeMutation = useResourceMutation({
    mutation: siteMembershipsDeleteMutation(),
    invalidate: [membersKey],
    errorTitle: t(($) => $.settings.members.removeError),
  })

  const leaveMutation = useResourceMutation({
    mutation: siteWorkspacesLeaveMutation(),
    invalidate: [siteWorkspacesListQueryKey()],
    successMessage: t(($) => $.settings.members.left),
    errorTitle: t(($) => $.settings.members.leaveError),
    // The index route sends the User to another Workspace, or to the create screen.
    onDone: () => navigate({ to: indexRoute.to }),
  })

  const resetMutation = useResourceMutation({
    mutation: siteMembershipsResetSecondFactorMutation(),
    invalidate: [membersKey],
    errorTitle: t(($) => $.settings.members.resetSecondFactorError),
  })

  const onResetSecondFactor = (id: string) =>
    confirmRemove({
      title: t(($) => $.settings.members.resetSecondFactorConfirmTitle),
      description: t(($) => $.settings.members.resetSecondFactorConfirmDescription),
      confirmLabel: t(($) => $.settings.members.resetSecondFactorConfirm),
      onConfirm: () => resetMutation.mutate({ path: { workspaceId, id } }),
    })

  const onLeave = () =>
    confirmRemove({
      title: t(($) => $.settings.members.leaveConfirmTitle),
      description: t(($) => $.settings.members.leaveConfirmDescription),
      confirmLabel: t(($) => $.settings.members.leaveConfirm),
      onConfirm: () => leaveMutation.mutate({ path: { workspaceId } }),
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
        <Group>
          <Button color="red" variant="light" onClick={onLeave}>
            {t(($) => $.settings.members.leave)}
          </Button>
          <Button onClick={openInvite}>{t(($) => $.settings.members.invite)}</Button>
        </Group>
      </Group>

      {membersQuery.isError ? (
        <Alert color="red" title={t(($) => $.settings.members.loadError)} mb="md">
          {t(($) => $.settings.members.loadError)}
        </Alert>
      ) : null}

      {membersQuery.isLoading ? <Loader /> : null}

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
                render: (record) => (
                  <Select
                    size="xs"
                    w={130}
                    allowDeselect={false}
                    data={MEMBER_ROLES}
                    value={record.role}
                    onChange={(value) => {
                      if (value && value !== record.role) {
                        roleMutation.mutate({
                          path: { workspaceId, id: record.id },
                          body: { role: value },
                        })
                      }
                    }}
                  />
                ),
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
                render: (record) => (
                  <Group justify="flex-end">
                    <RowActionsMenu>
                      {canManageMembers && record.userId !== me.data?.id ? (
                        <Menu.Item color="orange" onClick={() => onResetSecondFactor(record.id)}>
                          {t(($) => $.settings.members.resetSecondFactor)}
                        </Menu.Item>
                      ) : null}
                      <Menu.Item color="red" onClick={() => onRemove(record.id)}>
                        {t(($) => $.settings.members.remove)}
                      </Menu.Item>
                    </RowActionsMenu>
                  </Group>
                ),
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
                render: (record) => (
                  <Group justify="flex-end">
                    <RowActionsMenu>
                      <Menu.Item color="red" onClick={() => onRevoke(record.id)}>
                        {t(($) => $.settings.members.revoke)}
                      </Menu.Item>
                    </RowActionsMenu>
                  </Group>
                ),
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
