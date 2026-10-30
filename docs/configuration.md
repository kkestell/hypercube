# Configuration

Create `~/.config/hypercube/config.json`, or
`$XDG_CONFIG_HOME/hypercube/config.json` when that variable is non-empty:

```json
{
  "orchestrator": {
    "harness": "claude",
    "model": "opus",
    "effort": "high"
  },
  "profiles": [
    {
      "name": "fast",
      "description": "Small, well-specified changes and quick questions.",
      "harness": "claude",
      "model": "haiku",
      "effort": "low"
    },
    {
      "name": "opencode",
      "description": "Work assigned to an OpenCode worker.",
      "harness": "opencode",
      "model": "provider/model",
      "effort": "variant"
    }
  ]
}
```

All fields shown are required. Provide at least one worker profile with a
unique name. Replace the OpenCode placeholders with a full model ID from
`opencode models` and a variant supported by that model, or remove that
profile.

`orchestrator` selects the settings for new teams. `profiles` defines the
choices offered to the orchestrator when creating workers; descriptions
should explain which work suits each profile. Existing agents keep their
saved settings across restarts. Changes affect subsequently created agents,
and the available profiles are reread when tools and startup context are
requested.

Model and effort values are interpreted by the selected harness. Claude Code
receives them as its model and effort; OpenCode uses effort as the model's
variant. Consult the harness for accepted values. Hypercube's exact schema
and validation are maintained in [config.go](../internal/harness/config.go).

## Files needed by worker worktrees

A new managed worktree starts at the orchestrator's HEAD and initializes
submodules. Uncommitted changes in the original checkout are absent.

To supply ignored local files, add a `.worktreeinclude` file at the checkout
root using Git ignore patterns, for example:

```gitignore
.env
node_modules/
```

Only paths also ignored by Git are copied. Copies are independent APFS
clones, so workers can change them without modifying the originals. They are
made when the worktree is created, not kept in sync. The implementation is
in [workspace.go](../internal/workspace/workspace.go).
