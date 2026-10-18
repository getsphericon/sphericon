import {
  Alert,
  Button,
  Card,
  Code,
  CopyButton,
  Group,
  Loader,
  Select,
  TextInput,
  Title,
} from '@mantine/core'
import { useForm } from '@mantine/form'
import { useQuery } from '@tanstack/react-query'
import { DataTable } from 'mantine-datatable'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

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
import type { SiteInvitableRole, SiteMembershipRole } from '../../generated/site/types.gen.ts'
import { canManage, useCurrentRole } from '../../hooks/useCurrentRole.ts'
import { useDeleteConfirmation } from '../../hooks/useDeleteConfirmation.tsx'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { formatDate } from '../../utils/datetime.ts'

const MEMBER_ROLES: SiteMembershipRole[] = ['owner', 'admin', 'member']
const INVITABLE_ROLES: SiteInvitableRole[] = ['admin', 'member']

export function MembersSection({ slug }: { slug: string }) {
  const { t } = useTranslation()
  const confirmRemove = useDeleteConfirmation()
  const [inviteUrl, setInviteUrl] = useState<string | null>(null)
  const role = useCurrentRole(slug)
  const me = useQuery(siteUserGetMeOptions())
  const canManageMembers = canManage(role)

  const membersKey = siteMembershipsListQueryKey({ path: { slug } })
  const invitesKey = siteInvitationsListQueryKey({ path: { slug } })
  const membersQuery = useQuery(siteMembershipsListOptions({ path: { slug } }))
  const invitesQuery = useQuery(siteInvitationsListOptions({ path: { slug } }))

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
      onConfirm: () => resetMutation.mutate({ path: { slug, id } }),
    })

  const onRemove = (id: string) =>
    confirmRemove({
      title: t(($) => $.settings.members.removeConfirmTitle),
      description: t(($) => $.settings.members.removeConfirmDescription),
      onConfirm: () => removeMutation.mutate({ path: { slug, id } }),
    })

  const onRevoke = (id: string) =>
    confirmRemove({
      title: t(($) => $.settings.members.revokeConfirmTitle),
      description: t(($) => $.settings.members.revokeConfirmDescription),
      onConfirm: () => revokeMutation.mutate({ path: { slug, id } }),
    })

  return (
    <Card withBorder>
      <Title order={4} mb="sm">
        {t(($) => $.settings.members.title)}
      </Title>

      {membersQuery.isError ? (
        <Alert color="red" title={t(($) => $.settings.members.loadError)} mb="md">
          {t(($) => $.settings.members.loadError)}
        </Alert>
      ) : null}

      {membersQuery.isLoading ? <Loader /> : null}

      <Title order={5} mb="xs">
        {t(($) => $.settings.members.membersTitle)}
      </Title>
      <DataTable
        withTableBorder
        mb="lg"
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
                      path: { slug, id: record.id },
                      body: { role: value },
                    })
                  }
                }}
              />
            ),
          },
          {
            accessor: 'actions',
            title: '',
            render: (record) => (
              <Group gap="xs" wrap="nowrap" justify="flex-end">
                {canManageMembers && record.userId !== me.data?.id ? (
                  <Button
                    size="compact-sm"
                    color="orange"
                    variant="light"
                    onClick={() => onResetSecondFactor(record.id)}
                  >
                    {t(($) => $.settings.members.resetSecondFactor)}
                  </Button>
                ) : null}
                <Button
                  size="compact-sm"
                  color="red"
                  variant="light"
                  onClick={() => onRemove(record.id)}
                >
                  {t(($) => $.settings.members.remove)}
                </Button>
              </Group>
            ),
          },
        ]}
        noRecordsText={t(($) => $.settings.members.membersEmpty)}
      />

      <Title order={5} mb="xs">
        {t(($) => $.settings.members.invitationsTitle)}
      </Title>

      {inviteUrl ? (
        <Alert
          color="yellow"
          title={t(($) => $.settings.members.inviteUrlNotice)}
          mb="md"
          withCloseButton
          onClose={() => setInviteUrl(null)}
        >
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
      ) : null}

      <form
        onSubmit={form.onSubmit((values) =>
          inviteMutation.mutate({
            path: { slug },
            body: { email: values.email.trim(), role: values.role },
          }),
        )}
      >
        <Group align="flex-end" gap="sm" mb="md">
          <TextInput
            label={t(($) => $.settings.members.emailLabel)}
            type="email"
            required
            {...form.getInputProps('email')}
          />
          <Select
            label={t(($) => $.settings.members.roleLabel)}
            allowDeselect={false}
            data={INVITABLE_ROLES}
            w={140}
            {...form.getInputProps('role')}
          />
          <Button type="submit" loading={inviteMutation.isPending}>
            {t(($) => $.settings.members.invite)}
          </Button>
        </Group>
      </form>

      <DataTable
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
              <Button
                size="compact-sm"
                color="red"
                variant="light"
                onClick={() => onRevoke(record.id)}
              >
                {t(($) => $.settings.members.revoke)}
              </Button>
            ),
          },
        ]}
        noRecordsText={t(($) => $.settings.members.invitationsEmpty)}
      />
    </Card>
  )
}
