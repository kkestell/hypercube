---
title: Configuration
weight: 2
---

# Configuration

Hypercube reads `~/.config/hypercube/config.json`. If `XDG_CONFIG_HOME` is set,
the path is `$XDG_CONFIG_HOME/hypercube/config.json` instead. Without the file,
every preference takes its default.

Open Settings (`⌘,`) to choose the theme, terminal text size, defaults for new
sessions, guidance for choosing child models, and notifications. **Save
changes** validates and writes the configuration; closing the modal without
saving discards edits.

## Preferences

Settings writes these top-level fields. A file without them uses the defaults.

| Field | Value | Default |
| --- | --- | --- |
| `default_selection` | An object with `harness`, `model`, and `effort`: the settings of a new session in a workspace that has no sessions yet. Otherwise a new session starts with the settings of the workspace's newest session. An empty `effort` uses the harness's default. | none |
| `child_model_guidance` | Text that tells agents when a child session should use another harness, model, or effort than its parent. | empty |
| `default_worktree` | `true` to start new sessions in Git workspaces in a worktree. The composer can change it for each session. | `false` |
| `theme` | `system`, `light`, or `dark`. | `system` |
| `terminal_font_size` | The terminal's text size in pixels: 12, 13, 14, 16, or 18. | `13` |
| `notifications` | An object of four booleans. `requests` and `replies` choose which alerts appear as cards and macOS notifications; `macos` shows macOS notifications while the window is not focused; `sound` plays a sound with them. | all `true` |

Editing the file directly while Hypercube is running is unsupported. To edit
it by hand, stop Hypercube first; a settings save replaces the file.

## Choosing models

Hypercube asks each installed harness which models and efforts it offers, in
the folder where the session runs. The composer's selector lists them, and a
running session can change its model and effort within its harness. Accepted
model and effort values differ [by harness](harness-support.md#requirements).

A child session uses its parent's current harness, model, and effort unless the
parent passes others. Agents pass others when you ask for them, or when
`child_model_guidance` calls for them for that kind of task. The guidance is an
instruction to agents; it does not limit which models they can choose.

## Local files in worktrees

A new worktree contains committed files and initialized submodules. It may also
need ignored files from your checkout, such as environment configuration or
installed dependencies.

Put `.worktreeinclude` at the checkout root, using Git ignore patterns:

```gitignore
.env
node_modules/
```

Hypercube copies matching paths only if Git also ignores them. The copies are
independent copies made when the worktree is created. Where the file system
supports it (APFS on macOS, Btrfs and XFS on Linux), the copies are
copy-on-write clones; elsewhere they are ordinary copies. Changes in either
checkout do not update the other copy.

## Example configuration

```json
{
  "default_selection": {
    "harness": "opencode",
    "model": "opencode-go/deepseek-v4.1-flash",
    "effort": "max"
  },
  "child_model_guidance": "- Harness opencode, model opencode-go/deepseek-v4.1-flash, effort max: DeepSeek V4.1 Flash at max effort for ordinary implementation, investigation, and review. Cheap and fast. Artificial Analysis: Terminal-Bench 4.0 26.8%; no FrontierCode 2.0 or cost per task listed.\n- Harness codex, model gpt-6-luna, effort medium: GPT Luna 6 at medium effort only for trivial tasks such as lookups and quick checks. Weak at coding. Cheap and fast. Artificial Analysis: Terminal-Bench 4.0 2.5%, FrontierCode 2.0 12.8%, $0.04 per task.\n- Harness codex, model gpt-6.1-terra, effort high: GPT Terra at high effort for implementation, design work, and live skill testing.\n- Harness codex, model gpt-6.1-sol, effort high: GPT Sol at high effort, only for code review and for implementation from a written plan. Its reviews include many speculative findings, so verify each one before acting on it. Never use it for design work: it over-engineers. Artificial Analysis: Terminal-Bench 4.0 51.5%, FrontierCode 2.0 36.7%, $0.59 per task.\n- Harness codex, model gpt-6.1-sol, effort xhigh: GPT Sol at extra high effort, only for hard code review and for hard implementation from a written plan. Its reviews include many speculative findings, so verify each one before acting on it. Never use it for design work: it over-engineers. Slower and costlier. Artificial Analysis: Terminal-Bench 4.0 54.0%, FrontierCode 2.0 38.5%, $0.76 per task.\n- Harness claude, model haiku, effort xhigh: Haiku at extra high effort for small, well-defined tasks: lookups, simple edits, and quick checks. Artificial Analysis: Terminal-Bench 4.0 29.3%, FrontierCode 2.0 29.9%, $0.25 per task.\n- Harness claude, model sonnet, effort high: Sonnet at high effort for ordinary implementation, investigation, and review. Artificial Analysis: Terminal-Bench 4.0 43.9%, FrontierCode 2.0 25.7%, $1.64 per task.\n- Harness claude, model opus, effort medium: Opus at medium effort for hard implementation, debugging, and design work. Artificial Analysis: Terminal-Bench 4.0 52.5%, FrontierCode 2.0 33.1%, $2.45 per task.\n- Harness claude, model opus, effort high: Opus at high effort for the hardest design and debugging work. Slower and costlier than claude-large. Artificial Analysis: Terminal-Bench 4.0 56.6%, FrontierCode 2.0 35.9%, $3.46 per task.",
  "default_worktree": false,
  "theme": "system",
  "terminal_font_size": 13,
  "notifications": {
    "requests": true,
    "replies": true,
    "macos": true,
    "sound": true
  }
}
```
