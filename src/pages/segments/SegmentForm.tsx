import { Button, Group, Input, Stack, TextInput } from '@mantine/core'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { conditionCodec, draftText, emptyGroup } from '../../components/conditions/condition.ts'
import { ConditionBuilder } from '../../components/conditions/ConditionBuilder.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import type { SiteConditionGroup } from '../../generated/site/types.gen.ts'
import type { ResourceFormProps } from '../../resources/defineResource.tsx'
import { Route as workspaceRoute } from '../../routes/workspaces/$workspaceId/route.tsx'
import { Route as segmentsRoute } from '../../routes/workspaces/$workspaceId/segments/index.tsx'

// A Segment is always a Condition, so the payload carries one.
export interface SegmentPayload {
  name: string
  condition: SiteConditionGroup
}

export function SegmentForm({ form, isPending, onSubmit }: ResourceFormProps<SegmentPayload>) {
  const { t } = useTranslation()
  const { workspaceId } = workspaceRoute.useParams()
  // The form holds the Condition as JSON text; a blank or unreadable text is the empty group. The
  // builder starts from it and keeps its own state; the form gets every edit back as a draft, which
  // the schema refuses on submit while an event count is blank.
  const [initialCondition] = useState(() => {
    const parsed = conditionCodec.safeParse(form.values.condition)
    return parsed.success ? parsed.data : emptyGroup
  })

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
            initialValue={initialCondition}
            onChange={(next) => form.setFieldValue('condition', draftText(next))}
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
