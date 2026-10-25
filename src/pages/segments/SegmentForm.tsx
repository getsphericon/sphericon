import { Button, Group, Input, Stack, TextInput } from '@mantine/core'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { conditionCodec, emptyGroup } from '../../components/conditions/condition.ts'
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
  const condition = form.values.condition
  // The form holds the Condition as JSON text; a blank or unreadable text is the empty group.
  const group = useMemo(() => {
    const parsed = conditionCodec.safeParse(condition)
    return parsed.success ? parsed.data : emptyGroup
  }, [condition])

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
            value={group}
            onChange={(next) => form.setFieldValue('condition', z.encode(conditionCodec, next))}
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
