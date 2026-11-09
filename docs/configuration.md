# Configuration reference

Open **Settings** with **Command+,** on macOS or **Ctrl+,** on Linux, or with
the gear button at the top of the sidebar. It opens to **General**.
**Save changes** validates and writes preferences; closing without saving
discards edits. No configuration file is needed to use the defaults.
Settings has four sections: **General**, **Agents**, **Notifications**, and
**Permissions**. Each part below names where its settings are.

The file is `~/.config/hypercube/config.json`, or
`$XDG_CONFIG_HOME/hypercube/config.json` when that variable is set.
Direct file edits while Hypercube runs are unsupported: a Settings save
replaces the file. Stop Hypercube before editing by hand.
[An example configuration](https://github.com/kkestell/hypercube/blob/main/docs/examples/config.json)
shows every field.

## Session defaults

Set these in **Settings › General › New sessions**: **Default model** and
**Default location**.

| Field | Meaning | Default |
| --- | --- | --- |
| `default_selection` | Object with `harness`, `model`, and `effort`; used for a workspace with no sessions. Otherwise, the newest session supplies the initial selection. | Not set |
| `default_worktree` | Start new Git sessions in a worktree. The composer can override this for each session. | `false` |

A model with effort levels needs an `effort`; a model without them takes none.
Model and effort values are the harness's native IDs; the plugin rejects
invalid ones at start or model change.
See [harness requirements](harness-support.md#requirements).

## Models

Edit the model picker list in **Settings › Agents › Models**. Each entry needs
a harness, its exact model ID, and a display title. Add effort levels ordered
from lowest to highest and the effort a session starts with. Leave levels
empty for a model without an effort control.
One level is its own default; several levels require a default from the list.

The `models` field holds the list, empty by default. Each entry is an object
with `harness`, `model`, `title`, `efforts` (lowest first), and
`default_effort`. A harness lists each model once.

Entries are saved in `config.json` and apply on **Save changes**, like other
settings. Entries are checked against the harness only when a session starts
or changes model.
Editing or deleting an entry does not change existing session settings or saved
defaults. Agents can still discover and choose models that are not in this
list.

## Child sessions

Set **Guidance for choosing child models** in **Settings › Agents › Child
sessions**.

| Field | Meaning | Default |
| --- | --- | --- |
| `child_model_guidance` | Instructions to agents about when children should use a different harness, model, or effort. It does not restrict available choices. | Empty text |

## Appearance

Set **Theme** and **Terminal text size** in **Settings › General ›
Appearance**.

| Field | Values | Default |
| --- | --- | --- |
| `theme` | `system`, `light`, `dark` | `system` |
| `terminal_font_size` | Pixels: `12`, `13`, `14`, `16`, `18` | `13` |

## Notifications

Set these in **Settings › Notifications**. All four fields in the
`notifications` object default to `true`.

| Field | Setting | Effect |
| --- | --- | --- |
| `requests` | **An agent needs my attention** | Show question and approval alerts as cards and desktop notifications. |
| `replies` | **An agent has a new reply** | Show new-reply alerts as cards and desktop notifications. |
| `desktop` | **Show desktop notifications** | Deliver desktop notifications while the app is open but its window is not focused. |
| `sound` | **Play a sound** | Play a sound with desktop notifications. Available while desktop notifications are on. |

Notification delivery and dismissal are described in the
[interface reference](interface.md#notifications).

## Permissions

**Settings › Permissions › Always Allowed Tools** lists the tools you chose to
always allow from an approval, with each choice's harness and whether it
applies to one session or all sessions. **Remove** deletes a choice, so the
harness asks again when it needs approval. Removal applies immediately,
without **Save changes**.

Saved choices are kept in the [database](storage.md#hypercube-files), not in
`config.json`. [Permission behavior](harness-support.md#permissions) varies by
harness.

## `.worktreeinclude`

This optional file at the checkout root uses Git ignore patterns to select
ignored files to copy into **new** worktrees. Both conditions must hold: the
path matches `.worktreeinclude`, and Git ignores it.

New worktrees already contain committed files and initialized submodules.
Included local files are independent copies made at creation time, not links
or synchronized folders. Hypercube uses copy-on-write clones where supported
(APFS on macOS; Btrfs and XFS on Linux), otherwise ordinary copies.

For a setup example, see [Supply ignored local files](teamwork.md#supply-ignored-local-files).
