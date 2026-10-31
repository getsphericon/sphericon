import { Box, Stack, Switch, Text, Title } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import type { SiteWorkspaceResource } from '../../generated/site/types.gen.ts'
import { useDateFormat } from '../../hooks/useDateFormat.ts'
import {
  useLiftSecondFactorRequirement,
  useRequireSecondFactor,
} from '../../hooks/useSecondFactorRequirement.ts'
import { can } from '../../utils/permissions.ts'

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
  const { formatDate } = useDateFormat()
  const { t } = useTranslation()
  const canChange = can(workspace, 'second_factor:manage')
  const requiredAt = workspace.secondFactorRequiredAt
  const requireMutation = useRequireSecondFactor()
  const liftMutation = useLiftSecondFactorRequirement()

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
          disabled={!canChange || requireMutation.isPending || liftMutation.isPending}
          onChange={(event) =>
            (event.currentTarget.checked ? requireMutation : liftMutation).mutate({
              path: { workspaceId: workspace.id },
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
