import { Checkbox, Input, ScrollArea, Stack } from '@mantine/core'

// ScopePicker offers api token scopes ("resource:action") grouped by resource: a checkbox per
// resource that selects or clears all its actions, with the actions nested under it. The list
// scrolls on its own, so the form's button stays in view.
export function ScopePicker({
  label,
  options,
  value,
  onChange,
}: {
  label: string
  options: string[]
  value: string[]
  onChange: (scopes: string[]) => void
}) {
  const groups = Object.entries(Object.groupBy(options, (scope) => scope.split(':')[0] ?? scope))
  const selected = new Set(value)

  const toggle = (scopes: string[], on: boolean) => {
    const next = new Set(selected)
    for (const scope of scopes) {
      if (on) {
        next.add(scope)
      } else {
        next.delete(scope)
      }
    }
    // Keep the vocabulary's order, not the order of clicking.
    onChange(options.filter((scope) => next.has(scope)))
  }

  return (
    <Input.Wrapper label={label}>
      <ScrollArea.Autosize mah={320} mt="xs" type="auto">
        <Stack gap="xs">
          {groups.map(([resource, scopes = []]) => {
            const count = scopes.filter((scope) => selected.has(scope)).length
            return (
              <Stack key={resource} gap={4}>
                <Checkbox
                  label={resource}
                  size="sm"
                  fw={500}
                  checked={count === scopes.length}
                  indeterminate={count > 0 && count < scopes.length}
                  onChange={(event) => toggle(scopes, event.currentTarget.checked)}
                />
                <Stack gap={4} ml="xl">
                  {scopes.map((scope) => (
                    <Checkbox
                      key={scope}
                      label={scope.slice(resource.length + 1)}
                      size="sm"
                      checked={selected.has(scope)}
                      onChange={(event) => toggle([scope], event.currentTarget.checked)}
                    />
                  ))}
                </Stack>
              </Stack>
            )
          })}
        </Stack>
      </ScrollArea.Autosize>
    </Input.Wrapper>
  )
}
