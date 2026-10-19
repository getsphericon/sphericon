import { Box, Stack, Switch, Text, Title } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import type { SiteWorkspaceResource } from '../../generated/site/types.gen.ts'
import { canManage } from '../../hooks/useCurrentRole.ts'
import { useSetSecondFactorRequirement } from '../../hooks/useSetSecondFactorRequirement.ts'
import { formatDate } from '../../utils/datetime.ts'

// The grace a member without a Second factor gets (ADR 0020), counted from the later
// of the requirement's start and their Membership's creation.
const GRACE_MS = 7 * 24 * 60 * 60 * 1000

// SecondFactorRequirementSection shows the Workspace's Two-factor requirement and the
// grace it gives; an Owner or Admin can switch it on or off (the server refuses a
// Member).
export function SecondFactorRequirementSection({
  workspace,
}: {
  workspace: SiteWorkspaceResource
}) {
  const { t } = useTranslation()
  const canChange = canManage(workspace.role)
  const requiredAt = workspace.secondFactorRequiredAt
  const mutation = useSetSecondFactorRequirement()

  return (
    <Box>
      <Title order={4} mb="xs">
        {t(($) => $.secondFactorRequirement.settingTitle)}
      </Title>
      <Stack gap="sm">
        <Text c="dimmed" size="sm">
          {t(($) => $.secondFactorRequirement.settingDescription)}
        </Text>
        <Switch
          label={t(($) => $.secondFactorRequirement.switchLabel)}
          checked={Boolean(requiredAt)}
          disabled={!canChange || mutation.isPending}
          onChange={(event) =>
            mutation.mutate({
              path: { slug: workspace.slug },
              body: { required: event.currentTarget.checked },
            })
          }
        />
        {requiredAt ? (
          <Text size="sm">
            {t(($) => $.secondFactorRequirement.graceVisibility, {
              since: formatDate(requiredAt),
              until: formatDate(Date.parse(requiredAt) + GRACE_MS),
            })}
          </Text>
        ) : null}
      </Stack>
    </Box>
  )
}
