# Configuration

Create `~/.config/hypercube/config.json`. If `XDG_CONFIG_HOME` is set, the path
is `$XDG_CONFIG_HOME/hypercube/config.json` instead.

Open Settings (`⌘,`) to choose the theme, terminal text size, defaults for new
sessions, and notifications, or to add, edit, and remove profiles. **Save
changes** validates and writes the configuration; closing the modal without
saving discards edits. At least one complete
profile must remain. Renaming the default profile keeps it selected; removing
it selects the first remaining profile.

The optional top-level `default_profile` field names the profile selected for a
new session. If omitted in an existing file, it uses the first profile. Saving
from Settings writes an explicit default. The composer can override this
choice for each session.

After saving, an open session draft keeps its selected profile if that name
still exists; otherwise it switches to the saved default.

## Preferences

Settings writes these top-level fields. A file without them uses the defaults.

| Field | Value | Default |
| --- | --- | --- |
| `default_worktree` | `true` to start new sessions in Git workspaces in a worktree. The composer can change it for each session. | `false` |
| `theme` | `system`, `light`, or `dark`. | `system` |
| `terminal_font_size` | The terminal's text size in pixels: 12, 13, 14, 16, or 18. | `13` |
| `notifications` | An object of four booleans. `requests` and `replies` choose which alerts appear as cards and macOS notifications; `macos` shows macOS notifications while the window is not focused; `sound` plays a sound with them. | all `true` |

Editing the file directly while Hypercube is running is unsupported. To edit
it by hand, stop Hypercube first; a settings save replaces the file.

## Profiles

Each entry in `profiles` is a harness, model, and effort combination. It appears
in the selector below the message box. A parent agent also receives the list
when choosing how to run a child, so the description should tell it which work
the profile is suitable for.

All five fields are required. Names must be unique, and the file must contain
at least one profile.

| Field | Value |
| --- | --- |
| `name` | The name shown in the selector and used by agents. |
| `description` | When to choose this profile. |
| `harness` | `claude`, `opencode`, `codex`, or `pi`; its plugin must be installed. |
| `model` | A model ID accepted by the harness. |
| `effort` | A reasoning level or model variant accepted by that harness. |

Copy the [example below](#example-configuration) and remove profiles you do not
use. Accepted model and effort values differ
[by harness](harness-support.md#requirements).

Profile edits affect subsequent choices. An existing session keeps its saved
model and effort until you change its selection, even if you edit or remove
the original profile. Profiles are reread when the selector loads or opens and when
session tools or startup instructions request them.

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
independent APFS clones made when the worktree is created. Changes in either
checkout do not update the other copy.

## Example configuration

```json
{
  "profiles": [
    {
      "name": "opencode-deepseek-flash-max",
      "description": "DeepSeek V4.1 Flash at max effort for ordinary implementation, investigation, and review. Cheap and fast. Artificial Analysis: Terminal-Bench 4.0 26.8%; no FrontierCode 2.0 or cost per task listed.",
      "harness": "opencode",
      "model": "opencode-go/deepseek-v4.1-flash",
      "effort": "max"
    },
    {
      "name": "codex-gpt-luna-medium",
      "description": "GPT Luna 6 at medium effort only for trivial tasks such as lookups and quick checks. Weak at coding. Cheap and fast. Artificial Analysis: Terminal-Bench 4.0 2.5%, FrontierCode 2.0 12.8%, $0.04 per task.",
      "harness": "codex",
      "model": "gpt-6-luna",
      "effort": "medium"
    },
    {
      "name": "codex-gpt-terra-high",
      "description": "GPT Terra 6.1 at high effort for ordinary implementation, investigation, and review. Cheaper and faster than Sol high. Artificial Analysis: Terminal-Bench 4.0 41.9%, FrontierCode 2.0 37.5%, $0.26 per task.",
      "harness": "codex",
      "model": "gpt-6.1-terra",
      "effort": "high"
    },
    {
      "name": "codex-gpt-sol-high",
      "description": "GPT Sol at high effort, only for code review and for implementation from a written plan. Its reviews include many speculative findings, so verify each one before acting on it. Never use it for design work: it over-engineers. Artificial Analysis: Terminal-Bench 4.0 51.5%, FrontierCode 2.0 36.7%, $0.59 per task.",
      "harness": "codex",
      "model": "gpt-6.1-sol",
      "effort": "high"
    },
    {
      "name": "codex-gpt-sol-xhigh",
      "description": "GPT Sol at extra high effort, only for hard code review and for hard implementation from a written plan. Its reviews include many speculative findings, so verify each one before acting on it. Never use it for design work: it over-engineers. Slower and costlier. Artificial Analysis: Terminal-Bench 4.0 54.0%, FrontierCode 2.0 38.5%, $0.76 per task.",
      "harness": "codex",
      "model": "gpt-6.1-sol",
      "effort": "xhigh"
    },
    {
      "name": "claude-haiku-xhigh",
      "description": "Haiku at extra high effort for small, well-defined tasks: lookups, simple edits, and quick checks. Artificial Analysis: Terminal-Bench 4.0 29.3%, FrontierCode 2.0 29.9%, $0.25 per task.",
      "harness": "claude",
      "model": "haiku",
      "effort": "xhigh"
    },
    {
      "name": "claude-sonnet-high",
      "description": "Sonnet at high effort for ordinary implementation, investigation, and review. Artificial Analysis: Terminal-Bench 4.0 43.9%, FrontierCode 2.0 25.7%, $1.64 per task.",
      "harness": "claude",
      "model": "sonnet",
      "effort": "high"
    },
    {
      "name": "claude-opus-medium",
      "description": "Opus at medium effort for hard implementation, debugging, and design work. Artificial Analysis: Terminal-Bench 4.0 52.5%, FrontierCode 2.0 33.1%, $2.45 per task.",
      "harness": "claude",
      "model": "opus",
      "effort": "medium"
    },
    {
      "name": "claude-opus-high",
      "description": "Opus at high effort for the hardest design and debugging work. Slower and costlier than claude-opus-medium. Artificial Analysis: Terminal-Bench 4.0 56.6%, FrontierCode 2.0 35.9%, $3.46 per task.",
      "harness": "claude",
      "model": "opus",
      "effort": "high"
    }
  ]
}
```
