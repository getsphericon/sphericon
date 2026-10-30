import { Alert, Button, Group, NumberInput, Select, Skeleton, Stack, Title } from '@mantine/core'
import { useForm } from '@mantine/form'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import {
  siteWorkspacesGetRevenueSettingsOptions,
  siteWorkspacesGetRevenueSettingsQueryKey,
  siteWorkspacesUpdateRevenueSettingsMutation,
} from '../../../generated/site/@tanstack/react-query.gen.ts'
import type {
  SiteRevenueSettingsResource,
  SiteWorkspaceResource,
} from '../../../generated/site/types.gen.ts'
import { zAttributionWindowDays } from '../../../generated/site/zod.gen.ts'
import { useResourceMutation } from '../../../hooks/useResourceMutation.ts'
import { can } from '../../../utils/permissions.ts'

// The bounds the contract declares for the Attribution window, read off its generated schema.
const MIN_WINDOW_DAYS = zAttributionWindowDays.minValue ?? undefined
const MAX_WINDOW_DAYS = zAttributionWindowDays.maxValue ?? undefined

const CURRENCIES = Intl.supportedValuesOf('currency')

function RevenueSettingsForm({
  workspace,
  settings,
}: {
  workspace: SiteWorkspaceResource
  settings: SiteRevenueSettingsResource
}) {
  const { t } = useTranslation()
  const canEdit = can(workspace, 'revenue:configure')
  const form = useForm<{ currency: string | null; days: number }>({
    initialValues: {
      currency: settings.reportingCurrency,
      days: settings.attributionWindowDays,
    },
  })

  const mutation = useResourceMutation({
    mutation: siteWorkspacesUpdateRevenueSettingsMutation(),
    invalidate: [siteWorkspacesGetRevenueSettingsQueryKey({ path: { workspaceId: workspace.id } })],
    successMessage: t(($) => $.settings.saved),
    errorTitle: t(($) => $.settings.revenue.errorTitle),
    forbiddenMessage: t(($) => $.settings.revenue.forbidden),
  })

  return (
    <form
      onSubmit={form.onSubmit((values) =>
        mutation.mutate({
          path: { workspaceId: workspace.id },
          body: {
            ...(values.currency ? { reportingCurrency: values.currency } : {}),
            attributionWindowDays: values.days,
          },
        }),
      )}
    >
      <Stack maw={420}>
        <Select
          label={t(($) => $.settings.revenue.currencyLabel)}
          description={t(($) => $.settings.revenue.currencyDescription)}
          placeholder={t(($) => $.settings.revenue.currencyPlaceholder)}
          data={CURRENCIES}
          searchable
          allowDeselect={false}
          disabled={!canEdit}
          {...form.getInputProps('currency')}
        />
        <NumberInput
          label={t(($) => $.settings.revenue.windowLabel)}
          description={t(($) => $.settings.revenue.windowDescription)}
          min={MIN_WINDOW_DAYS}
          max={MAX_WINDOW_DAYS}
          allowDecimal={false}
          allowNegative={false}
          clampBehavior="strict"
          disabled={!canEdit}
          {...form.getInputProps('days')}
        />
        {canEdit ? (
          <Group justify="flex-end">
            <Button type="submit" loading={mutation.isPending}>
              {t(($) => $.actions.save)}
            </Button>
          </Group>
        ) : null}
      </Stack>
    </form>
  )
}

// RevenueSettingsSection holds the Workspace's Reporting currency and Attribution window
// (ADR 0064). Every member sees them; an Owner or Admin edits them (the server refuses a
// Member).
export function RevenueSettingsSection({ workspace }: { workspace: SiteWorkspaceResource }) {
  const { t } = useTranslation()
  const query = useQuery(
    siteWorkspacesGetRevenueSettingsOptions({ path: { workspaceId: workspace.id } }),
  )

  return (
    <Stack gap="xs">
      <Title order={4}>{t(($) => $.settings.revenue.title)}</Title>
      {query.isError ? (
        <Alert color="red" title={t(($) => $.settings.revenue.loadError)} />
      ) : query.data ? (
        <RevenueSettingsForm workspace={workspace} settings={query.data} />
      ) : (
        <Skeleton height={140} maw={420} />
      )}
    </Stack>
  )
}
