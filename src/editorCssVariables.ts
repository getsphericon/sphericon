import type { CSSVariablesResolver } from '@mantine/core'

// The message editor (@react-email/editor) styles itself from its own `--re-*` variables and
// follows the OS scheme, not Mantine's. Mapping them to Mantine's variables here makes it follow
// the app's light and dark scheme with no CSS of ours. Mantine emits these under
// `:root[data-mantine-color-scheme=...]`, which outranks the library's `:root` and `.dark`.
const editorVariables = {
  '--re-bg': 'var(--mantine-color-body)',
  '--re-bg-active': 'var(--mantine-color-default-hover)',
  '--re-border': 'var(--mantine-color-default-border)',
  '--re-separator': 'var(--mantine-color-default-border)',
  '--re-text': 'var(--mantine-color-text)',
  '--re-text-muted': 'var(--mantine-color-dimmed)',
  '--re-hover': 'var(--mantine-color-default-hover)',
  '--re-active': 'var(--mantine-color-gray-light-hover)',
  '--re-pressed': 'var(--mantine-color-gray-light-hover)',
  '--re-danger': 'var(--mantine-color-error)',
  '--re-danger-hover': 'var(--mantine-color-red-light-hover)',
  '--re-tooltip-bg': 'var(--mantine-color-text)',
  '--re-tooltip-text': 'var(--mantine-color-body)',
  '--re-radius': 'var(--mantine-radius-md)',
  '--re-radius-sm': 'var(--mantine-radius-sm)',
  '--re-shadow': 'var(--mantine-shadow-md)',
}

export const editorCssVariablesResolver: CSSVariablesResolver = () => ({
  variables: {},
  light: editorVariables,
  dark: editorVariables,
})
