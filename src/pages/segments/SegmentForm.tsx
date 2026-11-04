import { Button, Group, Input, Stack, TextInput } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { toCondition, emptyGroup } from '../../components/conditions/condition.ts'
import type { ConditionDraft } from '../../components/conditions/condition.ts'
import { ConditionBuilder } from '../../components/conditions/ConditionBuilder.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import type { SiteCreateSegmentInput } from '../../generated/site/types.gen.ts'
import type { ResourceFormProps } from '../../resources/defineResource.tsx'
import { Route as workspaceRoute } from '../../routes/workspaces/$workspaceId/route.tsx'
import { Route as segmentsRoute } from '../../routes/workspaces/$workspaceId/segments/index.tsx'

// What the form submits: the create input, which always carries a Condition (a Segment is one).
export type SegmentPayload = Pick<SiteCreateSegmentInput, 'name' | 'condition'>

// What the form holds: the create input, with the Condition as a draft the builder edits.
export type SegmentValues = Pick<SiteCreateSegmentInput, 'name'> & { condition: ConditionDraft }

export function SegmentForm({
  form,
  isPending,
  onSubmit,
}: ResourceFormProps<SegmentPayload, SegmentValues>) {
  const { t } = useTranslation()
  const { workspaceId } = workspaceRoute.useParams()

  return (
    <form onSubmit={form.onSubmit(onSubmit)}>
      <Stack>
        <TextInput
          label={t(($) => $.segments.nameLabel)}
          withAsterisk
          {...form.getInputProps('name')}
        />
        <Input.Wrapper
          label={t(($) => $.segments.conditionLabel)}
          description={t(($) => $.segments.conditionHint)}
        >
          <ConditionBuilder
            workspaceId={workspaceId}
            initialValue={toCondition(form.values.condition) ?? emptyGroup}
            onChange={(next) => form.setFieldValue('condition', next)}
          />
        </Input.Wrapper>

        <Group justify="flex-end">
          <ButtonLink variant="default" to={segmentsRoute.to} params={{ workspaceId }}>
            {t(($) => $.actions.cancel)}
          </ButtonLink>
          <Button type="submit" loading={isPending}>
            {t(($) => $.actions.save)}
          </Button>
        </Group>
      </Stack>
    </form>
  )
}
