import { Box, Button, Group, Modal, Paper, TextInput, useComputedColorScheme } from '@mantine/core'
import { useForm } from '@mantine/form'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  closeSnackbar,
  registerComponentDecorator,
  useStore,
  useWorkflowBuilderActions,
  WorkflowBuilder,
} from '@workflowbuilder/sdk'
import type {
  DidSaveStatus,
  IntegrationDataFormat,
  OnSaveExternal,
  WorkflowBuilderPlugin,
  WorkflowBuilderIsValidConnection,
} from '@workflowbuilder/sdk'
import i18next from 'i18next'
import { useCallback, useEffect, useLayoutEffect, useMemo, useState } from 'react'
import { I18nextProvider, useTranslation } from 'react-i18next'

import { ButtonLink } from '../../components/RouterLink.tsx'
import {
  siteAutomationsGetQueryKey,
  siteAutomationsListQueryKey,
  siteAutomationsUpdateMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteAutomationResource } from '../../generated/site/types.gen.ts'
import { useNotify } from '../../hooks/useNotify.ts'
import { i18n } from '../../i18n.ts'
import { Route as automationsRoute } from '../../routes/workspaces/$workspaceId/automations/index.tsx'
import { getApiErrorMessage } from '../../utils/apiErrors.ts'
import { graphToSteps, stepsToGraph } from './definition.ts'
import type { AutomationGraph } from './definition.ts'
import { FailedEnrollments } from './FailedEnrollments.tsx'
import { LeaveGuard } from './LeaveGuard.tsx'
import { automationSnapshot } from './leaveSnapshot.ts'
import { messageRenderer } from './messageRenderer.ts'
import { useAutomationNodeTypes } from './nodes.ts'
import { triggerRenderer } from './triggerRenderer.ts'

// Imported as a raw string (not auto-injected): the SDK sheet carries unlayered
// global resets and .mantine-* overrides that would leak app-wide. We mount it
// only while the builder is on screen and remove it on unmount, so it never
// touches the rest of the app (e.g. the dashboard's body scroll).
import builderStyles from '@workflowbuilder/sdk/style.css?inline'

// The email step's message is edited by the editor, not a text field, and the Trigger's event
// is typed with suggestions. A stable reference.
const jsonForm = { renderers: [messageRenderer, triggerRenderer] }

// The SDK ships a Templates button in the palette footer, for diagrams the model cannot hold.
// A wrapper decorator that renders nothing leaves the footer empty. A stable reference.
const withoutTemplates: WorkflowBuilderPlugin = () => {
  registerComponentDecorator('OptionalFooterContent', {
    place: 'wrapper',
    content: () => null,
    name: 'automationWithoutTemplates',
  })
}
const plugins = [withoutTemplates]

// The SDK announces that it restored the saved diagram on every mount, though the diagram was
// handed to it as props. Its snackbars are keyed `<variant>:<title>:<subtitle>`.
const RESTORED_SNACKBAR_KEY = 'success:restoreDiagramSuccess:'

function QuietRestore() {
  useEffect(() => {
    const timers = [0, 100, 400].map((delay) =>
      setTimeout(() => closeSnackbar(RESTORED_SNACKBAR_KEY), delay),
    )
    return () => {
      for (const timer of timers) {
        clearTimeout(timer)
      }
    }
  }, [])

  return null
}

// The linear model has one way in: nothing connects into the Trigger.
const isValidConnection: WorkflowBuilderIsValidConnection = ({ targetNode }) =>
  !targetNode.data.isStartNode

interface AutomationNameValues {
  name: string
}
type AutomationNameForm = ReturnType<typeof useAutomationNameForm>

function useAutomationNameForm(name: string) {
  const { t } = useTranslation()
  return useForm<AutomationNameValues>({
    initialValues: { name },
    validate: { name: (value) => (value.trim() === '' ? t(($) => $.validation.required) : null) },
  })
}

interface AutomationBuilderProps {
  workspaceId: string
  automation: SiteAutomationResource
}

// The SDK owns its own editor theme and only auto-detects it from
// prefers-color-scheme on first mount (reactFlowProps.colorMode is blocked to
// avoid clashing with it). That misses Mantine's manual ThemeToggle, whose
// choice is persisted independently. ThemeSync bridges the two: it reads the
// computed Mantine scheme (resolving "auto" + the persisted override) and
// drives the SDK's imperative setTheme, so the canvas follows the app. Must be
// a descendant of <WorkflowBuilder.Root> for the actions hook to resolve.
function ThemeSync() {
  const { setTheme } = useWorkflowBuilderActions()
  const scheme = useComputedColorScheme('light', { getInitialValueInEffect: true })

  useEffect(() => {
    setTheme(scheme)
  }, [scheme, setTheme])

  return null
}

// The bar floating over the canvas replaces the SDK's own top bar: the way back, the
// automation's name, the failed Enrollments and the Save that persists the canvas. It sits inside
// <WorkflowBuilder.Root> to reach the save action, and under the SDK's i18next instance, which
// does not hold the app's strings, so it is bound back to the app's instance.
function BuilderBar({
  form,
  workspaceId,
  automationId,
  onSave,
}: {
  form: AutomationNameForm
  workspaceId: string
  automationId: string
  onSave: (graph: AutomationGraph) => Promise<DidSaveStatus>
}) {
  const { t } = useTranslation()
  const saveMutation = useMutation({
    mutationFn: async () => {
      const { nodes, edges } = useStore.getState()
      return onSave({ nodes, edges })
    },
  })

  return (
    <Paper withBorder shadow="sm" p="xs" pos="absolute" top={16} left={16} right={16}>
      <Group justify="space-between" wrap="nowrap">
        <Group wrap="nowrap">
          <ButtonLink variant="default" to={automationsRoute.to} params={{ workspaceId }}>
            {t(($) => $.actions.back)}
          </ButtonLink>
          <TextInput
            aria-label={t(($) => $.automations.nameLabel)}
            placeholder={t(($) => $.automations.nameLabel)}
            w={320}
            {...form.getInputProps('name')}
          />
        </Group>
        <Group wrap="nowrap">
          <FailedEnrollments workspaceId={workspaceId} automationId={automationId} />
          <Button
            loading={saveMutation.isPending}
            onClick={() => {
              if (!form.validate().hasErrors) {
                saveMutation.mutate()
              }
            }}
          >
            {t(($) => $.actions.save)}
          </Button>
        </Group>
      </Group>
    </Paper>
  )
}

// AutomationBuilder hosts the visual flow editor. The name is edited in the bar over the canvas;
// the Trigger is the canvas's first node. Save drives onDataSave, which converts the graph to
// the trigger event and the linear []step definition and persists name, trigger and steps
// together through the generated update mutation.
export function AutomationBuilder({ workspaceId, automation }: AutomationBuilderProps) {
  const { t } = useTranslation()
  const notify = useNotify()
  const queryClient = useQueryClient()
  const nodeTypes = useAutomationNodeTypes()

  // Scope the SDK stylesheet to the builder's lifetime: present while mounted,
  // gone everywhere else. useLayoutEffect injects it before paint to avoid an
  // unstyled flash of the canvas.
  useLayoutEffect(() => {
    const style = document.createElement('style')
    style.dataset.workflowBuilder = ''
    style.textContent = builderStyles
    document.head.append(style)
    return () => style.remove()
  }, [])

  const form = useAutomationNameForm(automation.name)

  const initialGraph = useMemo(() => {
    const labels = Object.fromEntries(
      nodeTypes.flatMap((item) => ('type' in item ? [[item.type, item.label]] : [])),
    )
    return stepsToGraph(automation.triggerEvent, automation.steps, labels)
  }, [automation.triggerEvent, automation.steps, nodeTypes])

  const [saved, setSaved] = useState(() => automationSnapshot(automation.name, initialGraph))

  const updateMutation = useMutation({
    ...siteAutomationsUpdateMutation(),
    onSuccess: async (updated) => {
      await queryClient.invalidateQueries({
        queryKey: siteAutomationsListQueryKey({ path: { workspaceId } }),
      })
      await queryClient.invalidateQueries({
        queryKey: siteAutomationsGetQueryKey({ path: { workspaceId, id: updated.id } }),
      })
    },
  })

  const saveGraph = useCallback(
    async (graph: AutomationGraph): Promise<DidSaveStatus> => {
      const { triggerEvent, steps, dropped } = graphToSteps(graph)
      if (triggerEvent === null) {
        notify({
          color: 'red',
          title: t(($) => $.automations.triggerMissingTitle),
          message: t(($) => $.automations.triggerMissing),
        })
        return 'error'
      }
      if (dropped > 0) {
        notify({
          color: 'yellow',
          title: t(($) => $.automations.branchesUnsupportedTitle),
          message: t(($) => $.automations.branchesUnsupported),
        })
      }
      const values = form.getValues()
      try {
        await updateMutation.mutateAsync({
          path: { workspaceId, id: automation.id },
          body: {
            name: values.name.trim(),
            triggerEvent,
            steps,
          },
        })
        setSaved(automationSnapshot(values.name, graph))
        notify({
          color: 'teal',
          title: t(($) => $.notifications.successTitle),
          message: t(($) => $.notifications.automationUpdated),
        })
        return 'success'
      } catch (error) {
        notify({
          color: 'red',
          title: t(($) => $.alerts.automationSaveErrorTitle),
          message: getApiErrorMessage(
            error,
            t(($) => $.alerts.automationSaveErrorMessage),
          ),
        })
        return 'error'
      }
    },
    [workspaceId, automation.id, form, updateMutation, notify, t],
  )

  // The SDK's own save (its shortcut) answers with its success snackbar whatever this returns, so
  // the bar saves through `saveGraph` itself and only the app's toasts show.
  const onDataSave = useCallback<OnSaveExternal>(
    async (data: IntegrationDataFormat) => saveGraph({ nodes: data.nodes, edges: data.edges }),
    [saveGraph],
  )

  const integration = useMemo(() => ({ strategy: 'props' as const, onDataSave }), [onDataSave])

  // The SDK's canvas fills the viewport (100vw by 100vh), so the editor is a full-screen layer
  // over the app rather than a block inside a page. Its focus trap is off so the SDK's own
  // menus, rendered outside the dialog, stay usable.
  return (
    <Modal
      opened
      onClose={() => undefined}
      fullScreen
      padding={0}
      withCloseButton={false}
      closeOnEscape={false}
      trapFocus={false}
      transitionProps={{ duration: 0 }}
    >
      {/* The SDK initialises and reads the global i18next instance for its own
          strings. The app tree runs on a private instance (see main.tsx), so the
          builder subtree is bound back to the global one here. */}
      <I18nextProvider i18n={i18next}>
        {/* Children replace the SDK's default layout: its top bar (logo, folder name,
            language, theme and read-only toggles) is swapped for <BuilderBar />, and
            ThemeSync keeps the canvas theme in step with Mantine's color scheme. */}
        <WorkflowBuilder.Root
          key={automation.id}
          name={automation.name}
          layoutDirection="DOWN"
          nodeTypes={nodeTypes}
          initialNodes={initialGraph.nodes}
          initialEdges={initialGraph.edges}
          integration={integration}
          isValidConnection={isValidConnection}
          jsonForm={jsonForm}
          plugins={plugins}
        >
          <ThemeSync />
          <QuietRestore />
          <LeaveGuard name={form.values.name} saved={saved} />
          <Box pos="relative" w="100vw" h="100vh">
            <WorkflowBuilder.Canvas />
            <I18nextProvider i18n={i18n}>
              <BuilderBar
                form={form}
                workspaceId={workspaceId}
                automationId={automation.id}
                onSave={saveGraph}
              />
            </I18nextProvider>
            <Box pos="absolute" top={88} left={16}>
              <WorkflowBuilder.Palette />
            </Box>
            <Box pos="absolute" top={88} right={16}>
              <WorkflowBuilder.PropertiesPanel />
            </Box>
          </Box>
        </WorkflowBuilder.Root>
      </I18nextProvider>
    </Modal>
  )
}
