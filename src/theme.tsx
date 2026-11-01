import { createTheme, EmptyState } from '@mantine/core'
import { IconInbox } from '@tabler/icons-react'

// The app's Mantine theme. It starts as Mantine's defaults and grows one decision at a time:
// a color, a component default, a size. Both main.tsx and the test providers mount it, so a
// component renders the same in the app and in a test.
export const theme = createTheme({
  components: {
    // An empty list or report shows an icon in a neutral circle above its title, so it reads as a
    // placeholder and not as a stray line of text. A caller passes its own `icon` when one fits.
    // The icon is decoration: a screen reader reads the title alone.
    EmptyState: EmptyState.extend({
      defaultProps: {
        icon: <IconInbox />,
        withIndicatorBackground: true,
        py: 'xl',
        attributes: { indicator: { 'aria-hidden': true } },
      },
    }),
  },
})
