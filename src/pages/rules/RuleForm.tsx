import {
  ActionIcon,
  Button,
  Group,
  Input,
  Select,
  Stack,
  TagsInput,
  TextInput,
} from '@mantine/core'
import { IconTrash } from '@tabler/icons-react'
import { useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { ButtonLink } from '../../components/RouterLink.tsx'
import { siteEventsActionsOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteConditionGroup, SiteRuleAction } from '../../generated/site/types.gen.ts'
import type { ResourceFormProps } from '../../resources/defineResource.tsx'
import { Route as workspaceRoute } from '../../routes/workspaces/$workspaceId/route.tsx'
import { Route as rulesRoute } from '../../routes/workspaces/$workspaceId/rules/index.tsx'
import { conditionCodec, emptyGroup } from '../segments/condition.ts'
import { ConditionBuilder } from '../segments/ConditionBuilder.tsx'
import {
  actionsCodec,
  newAction,
  ruleActionKinds,
  takesTag,
  triggerEventsCodec,
} from './actions.ts'

export interface RulePayload {
  name: string
  triggerEvents: string[]
  condition: SiteConditionGroup
  actions: SiteRuleAction[]
}

export function RuleForm({ form, isPending, onSubmit }: ResourceFormProps<RulePayload>) {
  const { t } = useTranslation()
  const { workspaceId } = workspaceRoute.useParams()

  // The form holds each structured field as JSON text; a blank or unreadable text is empty.
  const triggerEvents = useMemo(() => {
    const parsed = triggerEventsCodec.safeParse(form.values.triggerEvents)
    return parsed.success ? parsed.data : []
  }, [form.values.triggerEvents])
  const condition = useMemo(() => {
    const parsed = conditionCodec.safeParse(form.values.condition)
    return parsed.success ? parsed.data : emptyGroup
  }, [form.values.condition])
  const actions = useMemo(() => {
    const parsed = actionsCodec.safeParse(form.values.actions)
    return parsed.success ? parsed.data : []
  }, [form.values.actions])

  const knownEvents = useQuery(siteEventsActionsOptions({ path: { workspaceId } }))
  const setActions = (next: SiteRuleAction[]) =>
    form.setFieldValue('actions', z.encode(actionsCodec, next))
  const changeAction = (index: number, change: Partial<SiteRuleAction>) =>
    setActions(actions.map((action, at) => (at === index ? { ...action, ...change } : action)))

  return (
    <form onSubmit={form.onSubmit(onSubmit)}>
      <Stack>
        <TextInput
          label={t(($) => $.rules.nameLabel)}
          withAsterisk
          {...form.getInputProps('name')}
        />
        <TagsInput
          label={t(($) => $.rules.triggerEventsLabel)}
          description={t(($) => $.rules.triggerEventsHint)}
          placeholder={t(($) => $.rules.triggerEventsPlaceholder)}
          withAsterisk
          data={knownEvents.data?.actions ?? []}
          value={triggerEvents}
          onChange={(next) =>
            form.setFieldValue(
              'triggerEvents',
              next.length > 0 ? z.encode(triggerEventsCodec, next) : '',
            )
          }
          error={form.errors.triggerEvents}
        />
        <Input.Wrapper
          label={t(($) => $.rules.conditionLabel)}
          description={t(($) => $.rules.conditionHint)}
        >
          <ConditionBuilder
            workspaceId={workspaceId}
            value={condition}
            onChange={(next) => form.setFieldValue('condition', z.encode(conditionCodec, next))}
          />
        </Input.Wrapper>
        <Input.Wrapper
          label={t(($) => $.rules.actionsLabel)}
          description={t(($) => $.rules.actionsHint)}
        >
          <Stack gap="xs" mt="xs">
            {actions.map((action, index) => (
              <Group key={index} align="flex-end" wrap="nowrap">
                <Select
                  label={t(($) => $.rules.actionTypeLabel)}
                  allowDeselect={false}
                  data={ruleActionKinds.map((type) => ({
                    value: type,
                    label: t(($) => $.rules.actionType[type]),
                  }))}
                  value={action.type}
                  onChange={(next) => {
                    const type = ruleActionKinds.find((kind) => kind === next)
                    if (type) {
                      changeAction(index, { type })
                    }
                  }}
                />
                {takesTag(action.type) ? (
                  <TextInput
                    label={t(($) => $.rules.actionTagLabel)}
                    value={action.tag ?? ''}
                    onChange={(event) => changeAction(index, { tag: event.currentTarget.value })}
                  />
                ) : null}
                <ActionIcon
                  variant="subtle"
                  color="red"
                  aria-label={t(($) => $.rules.actionRemove)}
                  onClick={() => setActions(actions.filter((_, at) => at !== index))}
                >
                  <IconTrash size={16} />
                </ActionIcon>
              </Group>
            ))}
            <Group>
              <Button
                variant="light"
                size="compact-sm"
                onClick={() => setActions([...actions, newAction()])}
              >
                {t(($) => $.rules.addAction)}
              </Button>
            </Group>
          </Stack>
        </Input.Wrapper>

        <Group justify="flex-end">
          <ButtonLink variant="default" to={rulesRoute.to} params={{ workspaceId }}>
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
