import { Box, Stack, Switch, Text, Title } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import type { SiteWorkspaceResource } from '../../generated/site/types.gen.ts'
import { useFormat } from '../../hooks/useFormat.ts'
import {
  useLiftSecondFactorRequirement,
  useRequireSecondFactor,
} from '../../hooks/useSecondFactorRequirement.ts'
import { can } from '../../utils/permissions.ts'

// SecondFactorRequirementSection shows the Workspace's Two-factor requirement and the
// grace it gives; an Owner or Admin can switch it on or off (the server refuses a
// Member).
export function SecondFactorRequirementSection({
  workspace,
}: {
  workspace: SiteWorkspaceResource
}) {
  const { formatDate } = useFormat()
  const { t } = useTranslation()
  const canChange = can(workspace, 'second_factor:manage')
  const requiredAt = workspace.secondFactorRequiredAt
  // The grace of the members there when the requirement began: the server's rule (ADR 0020).
  const graceEndsAt = workspace.secondFactorRequirementGraceEndsAt
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
        {requiredAt && graceEndsAt ? (
          <Text size="sm">
            {t(($) => $.secondFactorRequirement.graceVisibility, {
              since: formatDate(requiredAt),
              until: formatDate(graceEndsAt),
            })}
          </Text>
        ) : null}
      </Stack>
    </Box>
  )
}
