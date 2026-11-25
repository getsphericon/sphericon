import {
  ActionIcon,
  Box,
  Group,
  Menu,
  Paper,
  Text,
  ThemeIcon,
  UnstyledButton,
  useComputedColorScheme,
} from '@mantine/core'
import { IconPlus, IconTrash } from '@tabler/icons-react'
import type { Icon } from '@tabler/icons-react'
import { Background, Handle, Position, ReactFlow } from '@xyflow/react'
import type { NodeProps } from '@xyflow/react'
import { createContext, use, useMemo } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import type { EditedKind, Flow, FlowStep } from './definition.ts'
import { layoutFlow, TRIGGER_NODE_ID } from './flowLayout.ts'
import type { InsertNode, StepNode } from './flowLayout.ts'
import { insertableKinds, TRIGGER_ICON, useStepInfo, useStepKinds } from './stepKinds.ts'

// What the nodes of the canvas do: they select what the side panel edits, insert a Step at their
// place and remove theirs. Nothing on the canvas edits the flow but through these.
interface CanvasControls {
  flow: Flow
  editable: boolean
  selected: string | null
  select: (id: string) => void
  insert: (at: number, kind: EditedKind) => void
  remove: (id: string) => void
}

const Controls = createContext<CanvasControls | null>(null)

function useControls(): CanvasControls {
  const controls = use(Controls)
  if (controls === null) {
    throw new Error('A canvas node is rendered outside the canvas')
  }
  return controls
}

// A node's edges attach at its top and bottom; none is drawn or connected by hand.
function Handles() {
  return (
    <>
      <Handle type="target" position={Position.Top} isConnectable={false} />
      <Handle type="source" position={Position.Bottom} isConnectable={false} />
    </>
  )
}

// The box of the Trigger and of a Step: its icon, its kind and what it says beside it. A click
// opens it in the side panel.
function NodeBox({
  id,
  icon: NodeIcon,
  label,
  detail,
  children,
}: {
  id: string
  icon: Icon
  label: string
  detail: string
  children?: ReactNode
}) {
  const { selected, select } = useControls()
  return (
    <Paper
      withBorder
      shadow={selected === id ? 'md' : 'xs'}
      w="100%"
      h="100%"
      px="sm"
      bd={selected === id ? '2px solid var(--mantine-primary-color-filled)' : undefined}
    >
      <Group h="100%" wrap="nowrap" gap="xs">
        <UnstyledButton flex={1} miw={0} h="100%" onClick={() => select(id)}>
          <Group wrap="nowrap" gap="sm">
            <ThemeIcon variant="light" size="lg">
              <NodeIcon size={20} />
            </ThemeIcon>
            <Box miw={0}>
              <Text size="sm" fw={500} truncate>
                {label}
              </Text>
              {detail !== '' && (
                <Text size="xs" c="dimmed" truncate>
                  {detail}
                </Text>
              )}
            </Box>
          </Group>
        </UnstyledButton>
        {children}
      </Group>
    </Paper>
  )
}

function TriggerNodeView() {
  const { t } = useTranslation()
  const { flow } = useControls()
  return (
    <>
      <NodeBox
        id={TRIGGER_NODE_ID}
        icon={TRIGGER_ICON}
        label={t(($) => $.automations.stepTrigger)}
        detail={flow.trigger.action}
      />
      <Handle type="source" position={Position.Bottom} isConnectable={false} />
    </>
  )
}

// What a Step's node says beside its kind: an email's subject, a tag step's Tag.
function stepDetail(step: FlowStep): string {
  switch (step.kind) {
    case 'email':
      return step.draft.subject
    case 'apply_tag':
    case 'remove_tag':
      return step.draft.tag
    case 'telegram':
    case 'sms':
      return step.draft.text
    case 'http':
    case 'wait':
    case 'kept':
    default:
      return ''
  }
}

function StepNodeView({ data: { step } }: NodeProps<StepNode>) {
  const { t } = useTranslation()
  const { editable, remove } = useControls()
  const info = useStepInfo()(step)
  return (
    <>
      <Handles />
      <NodeBox id={step.id} icon={info.icon} label={info.label} detail={stepDetail(step)}>
        {/* Removing a single Step needs no confirmation (docs/standards/ui.md): it is quickly
            made again. */}
        {editable && step.kind !== 'kept' && (
          <ActionIcon
            variant="subtle"
            color="gray"
            aria-label={t(($) => $.automations.removeStep)}
            onClick={(event) => {
              // The node's own click opens it; removing it must not.
              event.stopPropagation()
              remove(step.id)
            }}
          >
            <IconTrash size={16} />
          </ActionIcon>
        )}
      </NodeBox>
    </>
  )
}

// An insertion point on an edge: it offers each kind of Step, and puts the one chosen there.
function InsertNodeView({ data: { at } }: NodeProps<InsertNode>) {
  const { t } = useTranslation()
  const { editable, insert } = useControls()
  const kinds = insertableKinds(useStepKinds())
  return (
    <>
      <Handles />
      {editable && (
        <Menu position="right-start" withinPortal>
          <Menu.Target>
            <ActionIcon
              variant="default"
              radius="xl"
              size={28}
              aria-label={t(($) => $.automations.insertStep)}
            >
              <IconPlus size={16} />
            </ActionIcon>
          </Menu.Target>
          <Menu.Dropdown>
            {kinds.map(({ kind, label, icon: KindIcon }) => (
              <Menu.Item
                key={kind}
                leftSection={<KindIcon size={16} />}
                onClick={() => insert(at, kind)}
              >
                {label}
              </Menu.Item>
            ))}
          </Menu.Dropdown>
        </Menu>
      )}
    </>
  )
}

// Stable references: the canvas re-registers its node kinds when this object changes.
const nodeTypes = { trigger: TriggerNodeView, step: StepNodeView, insert: InsertNodeView }

// FlowCanvas draws the flow top to bottom, laid out anew on every change (flowLayout.ts). Its nodes
// are neither dragged nor connected: the canvas cannot hold a shape the server would refuse.
export function FlowCanvas(controls: CanvasControls) {
  const { flow } = controls
  const { nodes, edges } = useMemo(() => layoutFlow(flow), [flow])
  const scheme = useComputedColorScheme('light', { getInitialValueInEffect: true })

  return (
    <Controls value={controls}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        colorMode={scheme}
        nodesDraggable={false}
        nodesConnectable={false}
        nodesFocusable={false}
        edgesFocusable={false}
        elementsSelectable={false}
        deleteKeyCode={null}
        // A click on the Trigger or a Step opens it in the side panel; an insertion point is no
        // node to open.
        onNodeClick={(_, node) => {
          if (node.type !== 'insert') {
            controls.select(node.id)
          }
        }}
        fitView
        fitViewOptions={{ maxZoom: 1 }}
      >
        <Background />
      </ReactFlow>
    </Controls>
  )
}
