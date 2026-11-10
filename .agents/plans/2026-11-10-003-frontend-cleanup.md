# Frontend cleanup

Fixes for the frontend audit of `internal/ui/web/` by two independent
reviewers (sessions `ui-audit-opus` and `ui-audit-sol`). The standard is the
"Code style" section of `docs/dev/development.md`. Paths below are relative to
`internal/ui/web/`; line numbers are from commit the work starts on and drift.

## Shared building blocks (already committed)

Use these everywhere they apply in your area. Do not change their interfaces
without telling the orchestrator; other workers use them at the same time.

- `shared/icon-button.tsx` — `IconButton({icon, label, shortcut?, variant?, ...buttonProps})`.
  Every icon-only button is this: stock `icon-xs` size (24px) with the 16px
  `Icon`, ghost by default, label as `aria-label` and `title`, shortcut keys
  appended to the title from `app/shortcuts.ts`. Delete local `size-6`,
  `size-7`, `size-4` overrides on icon buttons. It forwards `ref` and props, so
  it works under `asChild` triggers and with `aria-pressed`.
- `shared/message.tsx` — `Message({error?, fill?, className?})`. Every empty,
  loading, and error line shown in place of content, and every form error.
  `fill` is the centered whole-view message. Callers pass only layout spacing
  (`px-2`, `mb-4`) in `className`.
- `shared/error-text.ts` — `errorText(error)`, replacing every
  `error instanceof Error ? error.message : String(error)`.
- `shared/use-request.ts` — `useRequest(load | null, deps)` returning
  `{data, error, loading}`; replaces hand-written AbortController effects that
  load data for display.
- `shared/tones.ts` — the only status colors: `error`, `warning`, `info`,
  `success`, `muted`. Dots and bars use `bg-current` or `border-current` with a
  tone class. Red is for errors and for removed lines in diffs only.

## Decisions

- Diffs keep green for added and red for removed lines; the rule in
  `development.md` now says so.
- Usage limits: stock `Progress` with no internal overrides; the percentage
  text takes `tones.warning` at 75% or more.
- Keep the session title fade and the status dots' look (not part of this work).
- Keep `Icon`'s explicit `size-4`; with `icon-xs` it gives one 16px icon size.
- Cancel buttons are `outline`. Dialog footers use `DialogFooter`. Second-level
  headings are `text-lg font-semibold`, third-level `text-sm font-semibold`.
  Bordered lists are `divide-y rounded-md border`.
- Arbitrary values (`text-[11px]`, `rounded-[16px]`, pixel nudges) become the
  nearest theme step (`text-xs`, `rounded-2xl`).
- Hover-revealed controls fade opacity and visibility the way
  `sidebar/session-item.tsx` does. Content that appears in place fades in with
  the `animate-in fade-in` utilities shadcn already uses; content that opens
  and closes uses `slide` from `shared/motion.ts`.
- Collapsible content is never gated on `open`; Radix keeps it mounted through
  the closing slide. Dialogs and menus stay mounted with a controlled `open`,
  so their closing animation runs; keep their last content while closing.
- Every dialog returns focus to the control that opened it, through
  `DialogTrigger` or `shared/focus.ts`.
- Tests query by role, label, and text. Delete class names that exist only for
  tests or for DOM lookups. Delete a class only when nothing left in the repo
  uses it; the final pass removes leftovers that two areas shared.
- No interface text joins facts with separator characters (`·`, `•`).

## Area 1: app shell, sidebar, shared, editor, notifications (Opus, `ui-audit-opus`)

Owns `app/`, `sidebar/`, `shared/`, `editor/`, `notifications/`, `sessions/`,
`tesseract/`, `desktop/`, `base.css`, `store.ts`, `state.ts`, `main.tsx`, and
the tests in those folders. Also owns the listed lines in other areas.

1. Scroll memory keeps the position in a ref and writes the store only on
   unmount, `pagehide`, and hide (`editor/use-scroll-memory.ts:24,90`,
   `sidebar/sidebar.tsx:158-161`); a view moved into a split still reads the
   departing view's position. Move `use-scroll-memory.ts` to `shared/`; it
   serves the transcript and files too (update their imports).
2. Replace DOM lookups by class name with refs, data attributes the component
   owns, or state: `app/shortcuts.ts:101,117,131,161`, `agent/focus-composer.ts:3`,
   `agent/composer.tsx:40`, `app/app.tsx:33,87`, `editor/editor-tabs.tsx:26-28`,
   `sidebar/sidebar.tsx:74`. Next and previous session use a pure
   `sidebarRows(state)` shared with `SessionTree` (`sidebar.tsx:46-51`,
   `session-tree.tsx:44-57`). One way to focus file search
   (`files/search-panel.tsx:16-18` and `shortcuts.ts:117`).
3. Effects: drop the `quickOpen` reset effect (`app.tsx:59`); register
   document listeners once with a ref to the latest handler (`app.tsx:64-68,94-101`).
4. One owner for dialog open state: a `dialog` field in the store for settings,
   shortcuts, quick open, and session search, opened by named functions in
   `app/workbench.ts` (`openSettings(state, section)` and friends). Replace the
   `onSettings` and dialog props (`app.tsx:49-51,107`, `sidebar.tsx:31-35,115`)
   and the inline store writes at `agent/model-select.tsx:45`,
   `agent/working-context.tsx:23`, `app/shortcuts.ts:116`, and
   `changes/git-panel.tsx:50` (named functions for the right-panel tab and the
   git comparison).
5. Rename: `renaming` lives in the store; while renaming, keep only the frozen
   order of IDs, not copies of workspaces and sessions, and never write refs
   during render (`sidebar.tsx:39-53`, `workspace-section.tsx:15,58`,
   `session-tree.tsx:32-33,92`, `session-item.tsx:29-30`).
6. `shared/split.tsx:44-48`: clear `sliding` on `transitionend`, not a timer.
7. `notifications/errors.ts:8`: key with `crypto.randomUUID()`, delete `count`.
8. Reduced motion: `shared/busy-dot.tsx` and `tesseract/tesseract.tsx` do not
   animate when `prefers-reduced-motion: reduce` matches, including after the
   setting changes. Busy dot colors come from `tones.success`.
9. Motion: notification cards (`notification-stack.tsx:8`), the find bar
   (`shared/find.tsx:84`), and the tab drop overlay (`editor/editor-pane.tsx:64`)
   animate in. Delete `motion-reduce:transition-none` in `shared/icon.tsx:34`.
10. `Find` owns the floating overlay look by default with an `inline` option
    for the transcript; remove the class strings at `files/file-view.tsx:81`,
    `changes/diff-view.tsx:84`, and set `inline` at `transcript/transcript.tsx:77`.
    Rename `transcript-find-count`.
11. `TopBar` component for the three top-row tab bars (`editor/editor-tabs.tsx:30-31,40-49`,
    `editor/main-tab-bar.tsx:12-21`, `shared/panel-tabs.tsx:29-35`), owning the
    container, trailing actions, and the window-button clearance.
12. Status icons: `BranchIcon` and `PullRequestIcon` share one wrapper that sets
    the title once (`shared/branch-icon.tsx:18-20`, `github/pull-request-icon.tsx:10-12`);
    `StatusDot` and `BusyDot` share their outer box; `sessions/session-status.tsx`
    becomes a component; colors from `tones` (`branch-icon.tsx:9-10`,
    `status-dot.tsx:4-5`).
13. `useTreeFocus` in `shared/` for the focused row, tabbable fallback, focus
    repair, and arrow/Home/End keys of both trees (`sidebar/session-tree.tsx:58-88`,
    `files/files-panel.tsx:25,35-67`). Touch only that code in `files-panel.tsx`.
14. Styling: `sidebar.tsx:99` undefined `text-sidebar-foreground/70` becomes
    `text-muted-foreground`; `base.css:95` body text uses the theme foreground;
    `shared/tab-background.ts` is the one mixed surface (export it for
    `transcript/transcript-item.tsx`); remove `--workspace-surface`
    (`workspace-section.tsx:25-28,35,53`) and the custom shadow
    (`notification-card.tsx:64`); `text-[11px]` in `sidebar.tsx:96`,
    `workspace-section.tsx:44,49`; half-pixel offsets in `session-item.tsx:137-138`
    if the guide lines still align without them; Card used as a drop zone
    (`editor-pane.tsx:64`); restyled ghost buttons in `workspace-section.tsx:40,47`.
15. `IconButton` at every icon button in the area, including
    `app/layout-toggles.tsx` (titles and keys from `shortcuts.ts`, so
    "Toggle Sidebar", "Toggle Terminal", "Toggle Right Panel"),
    `editor/view-options.tsx:34,42,49`, `sidebar.tsx:117-118`,
    `session-search.tsx:46`, `workspace-section.tsx:47,54`, `find.tsx:92-94`,
    `notification-card.tsx`, and `editor-tabs.tsx:44`.
16. Settings: one model setting block instead of three
    (`app/settings-preferences.tsx:61-112`), with real `Label`s, a visible label
    for the utility model, and `DefaultSelection` inlined; `DialogFooter` and
    `Message` in `settings-dialog.tsx:65-80`; `settings-models.tsx:26` joins
    effort levels without `·`; settings dialogs and the models editor stay
    mounted and return focus (`settings-dialog.tsx:19-20`, `settings-models.tsx:20,27,32-34,73-74`);
    `shared/confirm-dialog.tsx:10-20` stays mounted. Use `useRequest` for
    `settings-dialog.tsx:31-39`, `settings-permissions.tsx:12-16`,
    `settings-models.tsx:53-59`, `sidebar/session-search.tsx:21-39`. Headings
    and lists per the decisions (`settings-dialog.tsx:65`,
    `workspace-dashboard.tsx:26`, `shortcuts-dialog.tsx:27`, `session-view.tsx:46`,
    `settings-preferences.tsx:34`, `settings-permissions.tsx:24-25`).
17. Delete `Stack` (`notification-stack.tsx:7-9`) and the `focusComposer`
    wrapper (`app.tsx:52`). Move header comments above imports where they sit
    below them in your files. `errorText` replaces the inline expressions in
    `api.ts` and the area.
18. `sidebar.tsx:122` context menu stays mounted for its closing animation.
19. Tests in the area query by role and label (for example
    `sidebar/sidebar.test.tsx`, `app/app.test.tsx`).

## Area 2: transcript and agent (Opus)

Owns `transcript/` and `agent/` and their tests, except the lines Area 1 owns
(`agent/model-select.tsx:45`, `agent/working-context.tsx:23`,
`agent/focus-composer.ts`, `agent/composer.tsx:40`, `transcript/transcript.tsx:77`).

1. Collapsible content not gated on `open`: `activity-item.tsx:24-25`,
   `tool-items.tsx:27-49`, `attention-item.tsx:35-46`, `message-item.tsx:77`.
2. Message previews never put links inside the disclosure button
   (`message-item.tsx:73-75`, `disclosure.tsx:25-29`, `markdown.tsx:83,119-123`);
   expanded messages keep working links.
3. Expansion state: disclosure rows read and write their own expansion in the
   store instead of `RowControls` drilled through the tree
   (`transcript.tsx:42-45,84,115`, `transcript-item.tsx:28-47`,
   `activity-item.tsx`, `tool-items.tsx`, `message-item.tsx`,
   `attention-item.tsx`, `proposal-item.tsx`).
4. `TranscriptDetails` component for the titled detail sections
   (`tool-items.tsx:32-41`, `attention-item.tsx:36-44`), with stock Card styling
   rather than borderless overrides (also `message-item.tsx:33`,
   `proposal-item.tsx:39`); Alert keeps its border (`transcript-item.tsx:26`).
5. `useSubmit` from `attention-form.tsx:43-51` replaces the copies in
   `task-offer.tsx:13-22` and `proposal-item.tsx:28-37`.
6. One tool-status function instead of two (`message-item.tsx:50-51`,
   `tool-items.tsx:21-23`). Move tool wording helpers from
   `transcript-tree.ts:18-92` into `tool-items.tsx` with their tests.
7. Delete the unused `title` and `preview` options of `Disclosure` if nothing
   uses them after item 2, and inline `ReasoningItem` (`reasoning-item.tsx`).
   Disclosure's trigger stops restyling the ghost button into a bare one
   where a plain `CollapsibleTrigger` button does the job
   (`disclosure.tsx:25`, `activity-item.tsx:19`).
8. Composer and pending requests (`agent/composer.tsx:89-128`,
   `transcript/transcript.tsx:79,103-119`): keep the connected look, but use
   theme tokens and `rounded-2xl`, no custom dark background, one shared padded
   column instead of the 37px, 11px, and 22px alignment numbers
   (`worktree-select.tsx:23` too), and a visible focus treatment (focus-within
   on the container) since the textarea's own is removed. The scroll-to-newest
   button (`transcript.tsx:98-101`) fades in.
9. `limits.tsx`: stock Progress (remove the `[&_[data-slot=…]]` overrides,
   `bg-neutral-500/20`, and the disabled transition), percentage in
   `tones.warning` at 75% or more, and the effect no longer depends on
   `Date.now()` every render (`limits.tsx:24-47`).
10. `use-mentions.tsx:71`: reset the selection in the handlers or derive it,
    not in an effect; `useRequest` for the file list (`use-mentions.tsx:55-65`).
11. `new-branch-dialog.tsx`: `useRequest`, `Message` for errors, outline
    Cancel, returns focus to the worktree menu; `worktree-select.tsx:31-43`
    uses the stock dropdown radio group and stays mounted while closing.
12. `model-select.tsx:46` stops restyling the command input border;
    `text-[10px]` at `:86` becomes `text-xs`. `copy-button.tsx:23` fades
    opacity and visibility. `transcript-item.tsx:19` uses the shared surface
    from `shared/tab-background.ts`.
13. `IconButton`, `Message`, `errorText`, and `tones` at every site in the area.
    Move header comments above imports (`composer.tsx`, `attention-form.tsx`,
    `message-item.tsx`, `tool-items.tsx`, `attention-item.tsx`, `transcript-item.tsx`).
14. Tests in the area query by role and label.

## Area 3: files, changes, GitHub (Sol)

Owns `files/`, `changes/`, `github/` and their tests, except the lines Area 1
owns (`files/files-panel.tsx` tree focus and keys code, `files/search-panel.tsx:16-18`,
the `Find` class strings at `files/file-view.tsx:81` and `changes/diff-view.tsx:84`,
`changes/git-panel.tsx:50`, `github/pull-request-icon.tsx`).

1. `files-panel.tsx:81-88` folders and `github/pull-request-groups.tsx:25-37`
   groups slide with `slide`; folder children are not gated on `expanded[path]`.
2. `pull-request-groups.tsx:27`: a stock Button trigger instead of the
   hand-copied focus ring.
3. `pull-request-row.tsx:44` and `pull-requests.tsx:29`: hover controls fade
   opacity and visibility. Row colors from `tones` (`pull-request-row.tsx:14-22`).
4. `changes/changes.ts:21` status colors from `tones`; added and removed counts
   look the same in `git-panel.tsx:31-33` and `diff-view.tsx:75-77`.
5. `diff-view.tsx:79-81`: `IconButton` with `shortcut="previousChange"` and
   `"nextChange"` instead of the hand-written key text.
6. `Message` for every panel and view message: `git-panel.tsx:19,54,64`,
   `files-panel.tsx:86,95,97,98`, `search-panel.tsx:30`, `quick-open.tsx:34`
   (errors as errors), `file-view.tsx:66`, `diff-view.tsx:27,86`,
   `browser-preview.tsx:8`, `pull-requests.tsx:33`, `all-pull-requests.tsx:30`,
   `pull-request-groups.tsx:22`.
7. Shared row recipe for tree rows (`files-panel.tsx:75`, `git-panel.tsx:24`,
   `search-panel.tsx:33`) and one changed-file label (`files-panel.tsx:76-78`,
   `git-panel.tsx:27-29`). `files-panel.tsx:74` title without `•`.
8. `GitHubError` takes the failure and is used by both
   `all-pull-requests.tsx:25-28` and `github-header.tsx:53-59`; "Checking
   GitHub." comes from one place (`all-pull-requests.tsx:30`, `pull-requests.tsx:15`).
9. `clone-dialog.tsx`: `Message` for errors, outline Cancel, returns focus to
   its opener, and stays mounted while closing (`all-pull-requests.tsx:29,36`).
   `path-menu.tsx:35-43` stays mounted while closing.
10. `useRequest` for `quick-open.tsx:19-27` and `search-panel.tsx:19-27`;
    `errorText` for `changes.ts:62,87`, `files.ts:136,154`, and the rest.
11. Tests in the area query by role and label.

## Done when

- `make check` passes (from the repository root).
- `make dev` shows no visible regressions in the touched areas.
- Every item above is done or reported back with the reason it was not.
