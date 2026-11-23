import { useStore } from '@workflowbuilder/sdk'

// The properties the builder's canvas store holds for the node of the id (the Trigger's is
// 'trigger', a Step's its Step's id), which is what the editor's Save reads. An edit in the
// properties panel reaches the store only after JSON Forms' debounce, a timer that a busy browser
// runs late: a test that saves right after an edit first waits here for the edit to arrive.
export function canvasProperties(id: string): Record<string, unknown> | undefined {
  const node = useStore.getState().nodes.find((candidate) => candidate.id === id)
  return node?.data.properties
}
