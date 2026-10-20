import { Button, Group, Input, Stack, TextInput } from '@mantine/core'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { ButtonLink } from '../../components/RouterLink.tsx'
import type { SiteSegmentRuleGroup } from '../../generated/site/types.gen.ts'
import type { ResourceFormProps } from '../../resources/defineResource.tsx'
import { Route as workspaceRoute } from '../../routes/workspaces/$workspaceId/route.tsx'
import { Route as segmentsRoute } from '../../routes/workspaces/$workspaceId/segments/index.tsx'
import { definitionCodec, emptyGroup } from './rules.ts'
import { SegmentRuleBuilder } from './SegmentRuleBuilder.tsx'

// A Segment is always a rule, so the payload carries a rule group.
export interface SegmentPayload {
  name: string
  definition: SiteSegmentRuleGroup
}

export function SegmentForm({ form, isPending, onSubmit }: ResourceFormProps<SegmentPayload>) {
  const { t } = useTranslation()
  const { workspaceId } = workspaceRoute.useParams()
  const definition = form.values.definition
  // The form holds the rule group as JSON text; a blank or unreadable text is the empty group.
  const group = useMemo(() => {
    const parsed = definitionCodec.safeParse(definition)
    return parsed.success ? parsed.data : emptyGroup
  }, [definition])

  return (
    <form onSubmit={form.onSubmit(onSubmit)}>
      <Stack>
        <TextInput
          label={t(($) => $.segments.nameLabel)}
          withAsterisk
          {...form.getInputProps('name')}
        />
        <Input.Wrapper
          label={t(($) => $.segments.rulesLabel)}
          description={t(($) => $.segments.definitionHint)}
        >
          <SegmentRuleBuilder
            workspaceId={workspaceId}
            value={group}
            onChange={(next) => form.setFieldValue('definition', z.encode(definitionCodec, next))}
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
