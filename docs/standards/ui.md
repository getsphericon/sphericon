# UI patterns

How a screen behaves: what it asks before a delete, when a form complains, what an empty list says.
The source is [Cloudscape](https://cloudscape.design/patterns), the only one (ADR 0091). We take its
rules and render them with Mantine; its components are not used. How a screen is styled is in
`CODING_STANDARDS.md` (no custom CSS, theming), where the code lives is in `src/AGENTS.md`.

Each rule names its Cloudscape page and the component that already follows it. **Target** marks a
rule the code does not follow yet: a new screen follows it, an existing one is brought in line by
the ticket the marker names. A screen that departs from a rule says so in its pull request.

## Deleting

[Delete patterns](https://cloudscape.design/patterns/resource-management/delete/). Pick the lowest
friction that fits all four of: how severe the deletion is, how many resources it touches, how
costly it is to recreate them, and whether something must be done first.

- **One click, no confirmation:** low risk, easy to recreate, nothing depends on it, no
  prerequisite. The row leaves the list and no message follows.
- **Simple confirmation:** one or several resources that are not quickly recreated but unlikely to
  break anything running. A modal that names the consequence (required, no default text); the toast
  after it reports the result. This is
  `useConfirmation` (`src/hooks/useConfirmation.tsx`) and, with the request, `useDeleteResource`.
- **Additional confirmation:** serious, irreversible or cascading: the person types the resource's
  name before the button enables. This is `DeleteWorkspaceControl` and `useConfirmation` with
  `typedName`, both rendering `TypedConfirmation`.

The modal says what the deletion does and what to do first (a deactivation, a copy), in its own
words rather than the default text. How a row is removed (physically or archived) is rule 9 of
`review-rules.md`, not a choice made here.

`useConfirmation` and `useDeleteResource` take the consequence text as required input (there is no
default) and `typedName` (the additional confirmation, the same `TypedConfirmation` component
`DeleteWorkspaceControl` renders). A credential or endpoint something live depends on (an API key,
a Webhook endpoint) takes the typed one.

## Forms and validation

[Validation](https://cloudscape.design/patterns/general/errors/validation/).

- A field is not validated when the page opens. It is validated when it loses focus after a value
  was entered, or left empty if required; once an error shows, it is re-checked on each keystroke.
  Everything is validated again on submit.
- An error about one field, from the browser or from the server (a name already taken), shows under
  that field. An error with no field (a Permission) is an `ApiErrorAlert` above the submit button.
  No "fix all errors" banner.
- On submit the page scrolls to the first error and focuses its field.
- The submit button stays enabled: it is how the person finds what is missing. It is disabled only
  when a prerequisite is missing, or in a confirmation modal.
- Put the format in the field's constraint text, not in the error, and not as a placeholder
  (Mantine `description`); the error says which requirement failed.
- Messages: "Name is required." / "Enter a valid email address." / "Name is too long."

A form built by `useSchemaForm` (`src/resources/schemaForm.ts`, the one place a schema form is
made) gets the blur and keystroke validation and the focus on the first invalid field from the hook;
a form needs no code of its own for them. A form on a bare `useForm` has to be moved to
`useSchemaForm` to get them; a field outside `getInputProps` gets no focus. A refusal from the
server reaches its fields the same way: the mutation's `onError` hands the error to
`form.showFieldErrors`, which shows each message under its field and focuses the first one, and
returns what no field of the form could show, for the page to show as an `ApiErrorAlert` or a toast.

## Lists and tables

[Table view](https://cloudscape.design/patterns/resource-management/view/table-view/) and
[global actions](https://cloudscape.design/patterns/general/actions/global-actions/).

- A list of resources is a `PagedTable` (`mantine-datatable`) on a route with a `page` search
  param. Columns run left to right by importance; a header does not change when a filter applies.
- A list that is the whole page is its page header (`pageHeader`): the collection's name is the
  title and the create button is at its right. The row under it holds the archived view, the
  filters and the count of what the view shows ("6 automations", the archived view counting its
  own), with the refresh button at its right; the table does not repeat the title above itself.
- When a click on the row opens the resource, the first column names it and is a link to it
  (`AnchorLink`), so it is reachable by keyboard and opens in a new tab. The link looks like the
  row's text (its color, weight 500, no underline): the whole row is the target, the name
  is not singled out. Numbers are right-aligned. Rows are striped.
- Header actions act on the collection or the selection; a row's actions are in its
  `RowActionsMenu`. One primary button per page. Order: view, edit, other, delete, create. A row
  offers the same state actions as the resource's page (Publish where that page enables it). Only
  an action that cannot be undone is red: Delete is, Archive (undone by Restore) is not.
- An archived view is a place of its own, not a filter: its empty state names it ("No archived
  automations") and offers no create action.
- Filters search the whole collection, not the loaded page. A text filter for a known value; a
  single-value filter per property for a few properties; a property filter (a query builder) when
  there are many.
- The search and the single-value filters sit in the toolbar row, after the archived view; each
  lives in the route's search, so the view is linkable, and a change starts again from the first
  page. A filter that matches nothing is zero results, not an empty list ("Empty states").
- A sortable column sorts the whole collection on the server. A column picked anew starts in its
  own direction (a name A to Z, a date the newest first); a second click turns it round. The
  default order is the absent search param, so the plain URL is the list as it first shows.
- Paging clears the selection; so do sorting, a filter and a change of columns.
- Selection (not built yet): a header checkbox selects the rows of the page only, and an action
  applies to those rows. A bulk delete confirms with the number of resources (rule above).
- Errors of an action are a toast (`useNotify`); an error loading the list is the `ApiErrorAlert`
  `PagedTable` shows above the table.
- `PagedTable` has a refresh button above the table and the time of the last successful refresh
  beside it, in a polite live region. The rows stay on screen while it runs, and a failed refresh
  leaves the previous time. No list adds its own refresh.

`automations/list.tsx` is the reference list: a new list, or one brought in line, follows it.

## Status and its details

[Status indicator](https://cloudscape.design/components/status-indicator/),
[popover](https://cloudscape.design/components/popover/) and
[tooltip](https://cloudscape.design/components/tooltip/).

- A resource shows one status, a word or two that names its current state ("Failing", "Disabled"),
  never color alone. When two could apply, the one the person must act on first wins.
- Why it is in that state (the reason, since when, the last error) opens in a popover on the
  status, not in a dimmed line under it. The popover is supplementary: the status alone is enough to
  act on. Its text is short, at most about 150 characters, with no links or buttons; a longer
  explanation or an action belongs on the page.
- A tooltip only names an icon, shows truncated text or explains a term. It holds no status
  details and nothing to act on.
- A dimmed line that is not about a status stays: a secondary value (an email under a name, a
  count under a figure) or an instruction.

This is `DetailsBadge` (`src/components/DetailsBadge.tsx`): a badge, and with `details` a button
that opens them in a popover, by click, tap or keyboard. Cloudscape draws a status as an icon and
colored text with a dashed-underline trigger; we keep Mantine's `Badge`, which the app uses
everywhere, and mark the badge that has details with an info icon.

A resource with a draft (ADR 0065) is Draft until it is first published, then Active or Inactive;
unpublished changes above a live version are a detail of that status, not a second badge
(`AutomationStatusBadge`).

## Empty states

[Empty states](https://cloudscape.design/patterns/general/empty-states/).

- **Empty** (no resource exists, or all were deleted) and **zero results** (a filter matches none)
  are two states. Zero results never invites the person to create something: `contacts/list.tsx`
  passes no empty-state text while a filter is on.
- A heading that names the state ("No contacts", "No matches"), bold, no end punctuation. A
  description only when it says why it is empty or what to do, below a heading, with end
  punctuation, never repeating the heading or the button.
- One action, a default-variant button: for empty, the same label as the list's create button; for
  zero results, "Clear filter".
- In Mantine: `EmptyState` (`@mantine/core`) with its `title` and `description`, and the action as
  a `Button variant="default"` inside `EmptyState.Actions`.
- The count shows with the collection ("Contacts (0)", or "0 automations" in a list with a page
  header).
- A list with nothing in it (its total is 0, from either state) shows the empty state in place of
  the table, centered in the page's free height, under the count. A departure from Cloudscape,
  which keeps it inside the table, so that an empty page does not read as a table with a strip at
  the top. A later page that comes back empty while the total is not keeps its table and
  pagination. An analytics report that is empty as a whole does the same; an empty chart beside
  other figures keeps the chart's height, its empty state in the middle.
- In code: `CenteredEmptyState` (`src/components/`). `DashboardShell` stretches the main area to
  the viewport as a column; the page's root `Stack` takes `flex={1}`, and so does every wrapper
  between it and the empty state, or the state stays at the top.
- An error is not an empty state: Cloudscape's empty-states page says to use an alert or a toast.
  (Its table-view page puts a fetch error in an alert inside the table's empty area; `PagedTable`
  puts the same `ApiErrorAlert` above the table. That is a departure, kept because the table keeps
  its place while a refresh fails.)
- An empty value in a cell or a key-value row is a hyphen, not a blank and not `0`.

`PagedTable` takes the anatomy as `collection` (the name in the count, "Contacts (0)") and `empty`
(`title`, an optional `description`, and the `action` button), and centers it when the list is
empty. A list that is not a `PagedTable` renders the same by hand (`suppressions/list.tsx`).

## Loading

[Loading and refreshing](https://cloudscape.design/patterns/general/loading-and-refreshing/).

- First load of a table is a skeleton with the columns' layout, not a spinner. A spinner is for
  loading more rows or a row's children.
- A query under `QueryBoundary` shows the skeleton, the error and the retry; a refresh keeps the
  current data on screen.
- Loading text has no article and no end punctuation ("Loading contacts").

## Errors

[Error messages](https://cloudscape.design/patterns/general/errors/error-messages/).

- Every message says what happened, why when it is known, and what to do. No raw machine text as
  the message. `ApiErrorAlert` shows a title and the API's own message; a development server's
  fault shows under it in `ProblemDebugPanel`, which a production Instance never sends.
- Where it shows: under the field (a field's own), an `ApiErrorAlert` near what failed (a form or
  a section), a toast (the outcome of an action), `AppErrorPage` or a section's boundary (a failure
  of the whole page or of one panel).
- Do not promise what we do not do ("our team was notified" only when something reports it), and do
  not blame the person.

## Unsaved changes

[Communicating unsaved changes](https://cloudscape.design/patterns/general/unsaved-changes/). A
form a person can leave half-typed renders `LeaveDialog`: it covers a link, Back and a closed tab
and names what would be lost. No "don't ask again". It is not shown when nothing changed or the
changes are saved.

## Editor sessions

An editor of a saved resource runs an editor session (`src/editing/editorSession.ts`, through
`useEditorSession`). The page hands it the mounted editor's current content to read and write; its
kind of resource supplies an adapter: how to fingerprint content, build a submission or refuse an
invalid one, fold an acknowledgement into content that has moved on, and its operations (Publish,
Revert, Send now, …). Whether the editor is unsaved, and whether an operation may start, comes from
the session, never from the page. The Automation editor runs one, with its canvas adapter in
`src/pages/automations/editorSession.ts`; so does the edit page of every resource described with
`defineResource`, with the schema-form adapter in `src/resources/resourceEditor.ts`, whose content is
the form's values, and the edit modal of a resource edited from its list (`defineModalEditor`: a
Custom field), which runs the same edit form and closes once its Save is acknowledged. A create page
or modal has no session. A list action that writes one field of a row (turning a Webhook on or off)
is no editor: it sends that field alone with the revision the row shows, and a refusal says why and
reads the list again.

- **Three contents.** The current content belongs to the editor: the form or the canvas. The
  submitted content is what a Save captured before its request. The acknowledged content is what
  the server confirmed; its fingerprint is taken from the submission, never rebuilt from the
  response, unless the session replaced the content with the server's normalized form of it (below).
  The editor is unsaved while the current content differs from the acknowledged one.
- **Edits stay enabled during Save.** An edit made while Save is pending stays, and stays unsaved
  after the acknowledgement. The other operations act on stored content, so the editor is
  read-only while one of them is pending, and one operation runs at a time: Publish, Revert, Send
  now and Schedule wait for Save. Publish waits for unsaved edits to be saved, and so do Send now
  and Schedule, which deliver the saved content. An operation's event may carry an input, such as
  the time Schedule sends at.
- **Only the session replaces content**: an explicit Revert, or a Save whose submission still
  equals the current content and which takes the server's normalized form of it, which is then the
  acknowledged content. Only then are the stateful field editors reloaded. Revert over unsaved edits
  asks first, in the leave dialog's words.
- **Leaving during Save** asks only when the current content differs from both the acknowledged and
  the submitted content: what the pending Save carries is not lost.
- **A query refresh updates metadata only**: a refetched resource changes the draft and status
  badges and what the operations allow, never the content.
- **A write names the revision it was made on** (ADR 0119). The session keeps a base revision: the
  one of the content it loaded, moved only when an acknowledgement is folded in or the session
  replaces the content, never by a refresh. Every write sends it. When a write is refused with
  `revision_changed`, or a refresh brings a newer revision than the base, the editor keeps its
  content as it is, still unsaved, and says the resource changed since it was opened
  (`src/editing/ChangedNotice.tsx`). It offers **Load the saved version**, which replaces the
  content and asks first over unsaved edits in the leave dialog's words, and **Save mine over it**,
  which sends the same content on the newer revision. Nothing is merged.
- **The acknowledgement is the write's success.** The cache write and the toast follow it; the
  invalidation of related lists starts after it and is not part of the operation, so a failed
  refetch never reads as a failed Save.
- **A write outlives its editor.** Leaving does not cancel a pending Save or operation: its
  request is never aborted, and its acknowledgement still writes the cache and refreshes the
  related lists. Nothing else happens once the editor closed: no toast, no change of content.

## Writing

Every string in the app, a message, a button, a hint:

- Sentence case; proper nouns keep their capitals. End punctuation on sentences, none on headings,
  buttons and labels. No exclamation points.
- Present tense, active voice, an action verb on a button ("Create contact", not "Contact creation").
- No "please", "thank you", ellipses, ampersands, "e.g.", "i.e." or "etc.".
- "Select" or "choose", not "click". "Previous" and "following", not "above" and "below".
- English only, a string in the `en` catalog (`src/AGENTS.md`).

## Not covered

Cloudscape has no pattern for Condition and Automation builders or for the Broadcast flow, and the
AWS-specific parts of it (service dashboards, density, card view, saved filter sets) are not used.
What is settled for those goes here when it is.
