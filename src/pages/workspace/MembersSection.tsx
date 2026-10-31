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
import { useSuspenseQueries } from '@tanstack/react-query'
import { DataTable } from 'mantine-datatable'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { QueryBoundary } from '../../components/QueryBoundary.tsx'
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
import type {
  SiteInvitableRole,
  SiteMembershipRole,
  SiteWorkspaceResource,
} from '../../generated/site/types.gen.ts'
import { zSiteInvitableRole, zSiteMembershipRole } from '../../generated/site/zod.gen.ts'
import { useConfirmation } from '../../hooks/useConfirmation.tsx'
import { useDateFormat } from '../../hooks/useDateFormat.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { can } from '../../utils/permissions.ts'

const MEMBER_ROLES = zSiteMembershipRole.options
const INVITABLE_ROLES = zSiteInvitableRole.options

// MembersTabs lists the members and the pending invitations; the section's boundary shows the
// load and its failure.
function MembersTabs({
  workspaceId,
  canManage,
  canResetSecondFactor,
  roleChange,
  onChangeRole,
  onResetSecondFactor,
  onRemove,
  onRevoke,
}: {
  workspaceId: string
  canManage: boolean
  canResetSecondFactor: boolean
  // The Role change being saved, if any.
  roleChange: { id: string; role: SiteMembershipRole } | null
  onChangeRole: (id: string, role: SiteMembershipRole) => void
  onResetSecondFactor: (id: string) => void
  onRemove: (id: string) => void
  onRevoke: (id: string) => void
}) {
  const { formatDate } = useDateFormat()
  const { t } = useTranslation()
  const [{ data: me }, { data: members }, { data: invites }] = useSuspenseQueries({
    queries: [
      siteUserGetMeOptions(),
      siteMembershipsListOptions({ path: { workspaceId } }),
      siteInvitationsListOptions({ path: { workspaceId } }),
    ],
  })

  return (
    <Tabs defaultValue="members" keepMounted={false}>
      <Tabs.List mb="md">
        <Tabs.Tab value="members">{t(($) => $.settings.members.membersTitle)}</Tabs.Tab>
        <Tabs.Tab
          value="invitations"
          rightSection={invites.length > 0 ? <Badge size="sm">{invites.length}</Badge> : null}
        >
          {t(($) => $.settings.members.invitationsTitle)}
        </Tabs.Tab>
      </Tabs.List>

      <Tabs.Panel value="members">
        <DataTable
          minHeight={members.length > 0 ? 0 : 200}
          withTableBorder
          records={members}
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
                const changing = roleChange?.id === record.id
                return (
                  <Select
                    size="xs"
                    w={130}
                    allowDeselect={false}
                    data={MEMBER_ROLES}
                    value={changing ? roleChange.role : record.role}
                    disabled={changing}
                    rightSection={changing ? <Loader size={14} /> : undefined}
                    onChange={(value) => {
                      if (value && value !== record.role) {
                        onChangeRole(record.id, value)
                      }
                    }}
                  />
                )
              },
            },
            {
              accessor: 'source',
              title: t(($) => $.settings.members.sourceLabel),
              render: ({ source }) => (source ? t(($) => $.settings.members.source[source]) : null),
            },
            {
              accessor: 'actions',
              title: '',
              render: (record) => {
                const canReset = canResetSecondFactor && record.userId !== me.id
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
          minHeight={invites.length > 0 ? 0 : 200}
          withTableBorder
          records={invites}
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
  )
}

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
  const canManage = can(workspace, 'members:manage')
  const canResetSecondFactor = can(workspace, 'second_factor:manage')

  const membersKey = siteMembershipsListQueryKey({ path: { workspaceId } })
  const invitesKey = siteInvitationsListQueryKey({ path: { workspaceId } })

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

      <QueryBoundary
        fallback={<TableSkeleton />}
        errorTitle={t(($) => $.settings.members.loadError)}
      >
        <MembersTabs
          workspaceId={workspaceId}
          canManage={canManage}
          canResetSecondFactor={canResetSecondFactor}
          roleChange={
            roleMutation.isPending
              ? { id: roleMutation.variables.path.id, role: roleMutation.variables.body.role }
              : null
          }
          onChangeRole={(id, role) =>
            roleMutation.mutate({ path: { workspaceId, id }, body: { role } })
          }
          onResetSecondFactor={onResetSecondFactor}
          onRemove={onRemove}
          onRevoke={onRevoke}
        />
      </QueryBoundary>

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
