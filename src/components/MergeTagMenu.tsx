import { Menu, Text } from '@mantine/core'

// MergeTagMenu lists the merge tags a text binds, by name, and inserts the one picked.
export function MergeTagMenu({
  label,
  tags,
  onInsert,
}: {
  label: string
  tags: readonly string[]
  onInsert: (tag: string) => void
}) {
  return (
    <Menu>
      <Menu.Target>
        <Text component="button" type="button" size="sm" c="blue">
          {label}
        </Text>
      </Menu.Target>
      <Menu.Dropdown>
        {tags.map((tag) => (
          <Menu.Item key={tag} onClick={() => onInsert(tag)}>
            {tag}
          </Menu.Item>
        ))}
      </Menu.Dropdown>
    </Menu>
  )
}
