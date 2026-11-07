---
title: Interface reference
weight: 32
---

# Interface reference

Look up controls, indicators, and limits here. For procedures, use
[Manage sessions](usage.md) or [Review files and changes](review-changes.md).

## Keyboard shortcuts

**Help → Keyboard Shortcuts** lists all bindings, including sidebar navigation.
Text fields retain their editing keys.

| Action | macOS | Linux |
| --- | --- | --- |
| New session | Command+N | Ctrl+N |
| Search conversations | Command+K | Ctrl+K |
| Next / previous session | Command+Option+Down / Up | Ctrl+PageDown / PageUp |
| Focus message box | Command+L | Ctrl+L |
| Find a file | Command+P | Ctrl+P |
| Search project contents | Command+Shift+F | Ctrl+Shift+F |
| Find in a file or diff | Command+F | Ctrl+F |
| Next / previous diff change | Option+F5 / Shift+Option+F5 | F7 / Shift+F7 |
| Toggle word wrap | Option+Z | Alt+Z |
| Toggle sidebar | Command+B | Ctrl+B |
| Toggle terminal | Command+J | Ctrl+J |
| Toggle right panel | Command+Option+B | Ctrl+Alt+B |
| Settings | Command+, | Ctrl+, |
| Keyboard shortcuts | Command+/ | Ctrl+/ |

On macOS, desktop menu shortcuts also work in text fields, terminals, and
dialogs. The terminal otherwise keeps its keys, except Quick Open, Search
Files, and Settings. On Linux, terminal copy/paste uses **Ctrl+Shift+C** /
**Ctrl+Shift+V**.

## Prompts and attachments

| Control or limit | Behavior |
| --- | --- |
| Enter | Send the prompt. |
| Shift+Enter | Add a line. |
| `@` followed by a path | Show project-file suggestions; Up/Down selects, Enter or Tab inserts the path. |
| Up on the message box's first line | Recall an earlier prompt for editing. Down on the last line moves forward through history to the draft. |
| Escape in chat / Stop in an empty message box | Interrupt the current turn. |
| Image formats | PNG, JPEG, GIF, WebP. |
| Image size | At most 10 MiB total per message; multiple images and image-only prompts are supported. |
| Draft text | Survives reloads, including typed answers and a new session's first prompt. |
| Draft images | Retained across session switches, not app reloads. |

## Sidebar states

| Indicator | Meaning |
| --- | --- |
| Breathing dot | Agent is working. |
| Amber dot | An answer is needed. |
| Blue dot | Unseen reply. |
| Gray dot | Idle. |
| Dimmed outline | Ended. |
| Hollow dot | Child session; its state is shown with the same conventions. |

Children are grouped under their parent. An active parent's ended children
are hidden until **Show Ended Sessions** is chosen in its menu.

## Notifications

- Request cards can be answered directly, or opened with the eye button.
  Closing one hides the card without answering.
- Reply cards close after 15 seconds or when the agent works again. Hover or
  keyboard focus pauses expiry. Closing a card does not mark the reply seen.
- Desktop notifications arrive while the desktop app is open and its window
  lacks focus. Clicking one opens its session.
- Alerts that arrive while the app is closed are not replayed later. The badge
  and interface show current requests when it opens.

**Settings → Notifications** controls request alerts, reply alerts, desktop
delivery, and sound. See [configuration fields](configuration.md#notifications).

## Usage indicators

The ring below the message box shows reported context use. Hover for token
counts and reported cost; available readings
[depend on the harness](harness-support.md#usage-reports).

Subscription usage shows the shortest reported window normally, and the
longest on hover or keyboard focus. A single reported window stays visible.
Wide panes also show time until reset. The meter's mark indicates elapsed time
within the window. After reset, the last reading is dimmed and marked
**Waiting** until the harness reports again.

## Files and search limits

| Feature | Scope or limit |
| --- | --- |
| Text preview | 256 KiB. |
| HTML preview | Relative resources load; scripts are disabled. |
| HTML/SVG source | Available for supported text encodings within the text limit. |
| Media | Images, audio, video, and PDFs open in the app, without the text limit. Some formats and codecs may not play. |
| Changes | Whole Git repository containing the session directory. |
| Quick Open and project search | In Git: tracked and untracked files, excluding ignored and hidden paths. |
| Project text search | Case-insensitive plain text; skips files over 256 KiB; stops at 1,000 matching lines. |

## Saved layouts

Each session remembers tabs, editor splits, open files, expanded items, scroll
positions, and terminal visibility across switches and restarts. The app also
remembers overall panel sizes, sidebar choices, and the last selected session.
