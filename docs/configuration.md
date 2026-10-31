# Configuration

Create `~/.config/hypercube/config.json`, or
`$XDG_CONFIG_HOME/hypercube/config.json` when that variable is non-empty:

```json
{
  "lead": {
    "harness": "claude",
    "model": "opus",
    "effort": "high"
  },
  "profiles": [
    {
      "name": "deepseek",
      "description": "DeepSeek V4.1 Flash at max effort for ordinary implementation, investigation, and review. Cheap and fast. Artificial Analysis: Terminal-Bench 4.0 26.8%; no FrontierCode 2.0 or cost per task listed.",
      "harness": "opencode",
      "model": "opencode-go/deepseek-v4.1-flash",
      "effort": "max"
    },
    {
      "name": "glm-flash",
      "description": "GLM-5.3-Flash at max effort for ordinary implementation, investigation, and review. Cheap and fast. Artificial Analysis: Terminal-Bench 4.0 32.8%, FrontierCode 2.0 17.6%, $0.58 per task.",
      "harness": "opencode",
      "model": "opencode-go/glm-5.3-flash",
      "effort": "max"
    },
    {
      "name": "muse-spark",
      "description": "Muse Spark 1.3 Contributor at high effort for ordinary implementation and investigation. Artificial Analysis: no High variant listed; at Xhigh, Terminal-Bench 4.0 16.7%; at Max, Terminal-Bench 4.0 33.3%, FrontierCode 2.0 23.1%, $2.39 per task.",
      "harness": "opencode",
      "model": "opencode-go/muse-spark-1.3-contributor",
      "effort": "high"
    },
    {
      "name": "mimo-flash",
      "description": "MiMo-V2.6-Flash for small, well-defined tasks: lookups, simple edits, and quick checks. Cheap. Artificial Analysis: Terminal-Bench 4.0 22.7%, FrontierCode 2.0 21.6%, $0.13 per task.",
      "harness": "opencode",
      "model": "opencode-go/mimo-v2.6-flash",
      "effort": "default"
    },
    {
      "name": "beam-free",
      "description": "Beam Free at medium effort, a very small free model for basic utility tasks: lookups, file searches, and simple mechanical edits. Artificial Analysis: Terminal-Bench 4.0 3.0%, FrontierCode 2.0 5.5%, $0.28 per task at list price.",
      "harness": "opencode",
      "model": "opencode-go/beam-free",
      "effort": "medium"
    },
    {
      "name": "iceman-free",
      "description": "Iceman Contributor Free, a very small free model for basic utility tasks: lookups, file searches, and simple mechanical edits. Artificial Analysis: not listed.",
      "harness": "opencode",
      "model": "opencode-go/iceman-contributor-free",
      "effort": "default"
    },
    {
      "name": "gpt-luna",
      "description": "GPT Luna 6 at medium effort only for trivial tasks such as lookups and quick checks. Weak at coding. Cheap and fast. Artificial Analysis: Terminal-Bench 4.0 2.5%, FrontierCode 2.0 12.8%, $0.04 per task.",
      "harness": "opencode",
      "model": "openai/gpt-6-luna",
      "effort": "medium"
    },
    {
      "name": "gpt-medium",
      "description": "GPT Sol at high effort, only for code review and for implementation from a written plan. Its reviews include many speculative findings, so verify each one before acting on it. Never use it for design work: it over-engineers. Artificial Analysis: Terminal-Bench 4.0 51.5%, FrontierCode 2.0 36.7%, $0.59 per task.",
      "harness": "opencode",
      "model": "openai/gpt-6.1-sol",
      "effort": "high"
    },
    {
      "name": "gpt-large",
      "description": "GPT Sol at extra high effort, only for hard code review and for hard implementation from a written plan. Its reviews include many speculative findings, so verify each one before acting on it. Never use it for design work: it over-engineers. Slower and costlier. Artificial Analysis: Terminal-Bench 4.0 54.0%, FrontierCode 2.0 38.5%, $0.76 per task.",
      "harness": "opencode",
      "model": "openai/gpt-6.1-sol",
      "effort": "xhigh"
    },
    {
      "name": "claude-small",
      "description": "Haiku at extra high effort for small, well-defined tasks: lookups, simple edits, and quick checks. Artificial Analysis: Terminal-Bench 4.0 29.3%, FrontierCode 2.0 29.9%, $0.25 per task.",
      "harness": "claude",
      "model": "haiku",
      "effort": "xhigh"
    },
    {
      "name": "claude-medium",
      "description": "Sonnet at high effort for ordinary implementation, investigation, and review. Artificial Analysis: Terminal-Bench 4.0 43.9%, FrontierCode 2.0 25.7%, $1.64 per task.",
      "harness": "claude",
      "model": "sonnet",
      "effort": "high"
    },
    {
      "name": "claude-large",
      "description": "Opus at medium effort for hard implementation, debugging, and design work. Artificial Analysis: Terminal-Bench 4.0 52.5%, FrontierCode 2.0 33.1%, $2.45 per task.",
      "harness": "claude",
      "model": "opus",
      "effort": "medium"
    },
    {
      "name": "claude-opus-high",
      "description": "Opus at high effort for the hardest design and debugging work. Slower and costlier than claude-large. Artificial Analysis: Terminal-Bench 4.0 56.6%, FrontierCode 2.0 35.9%, $3.46 per task.",
      "harness": "claude",
      "model": "opus",
      "effort": "high"
    },
    {
      "name": "claude-fable",
      "description": "Fable at high effort. Artificial Analysis: Terminal-Bench 4.0 52.0%, FrontierCode 2.0 30.3%, $7.54 per task.",
      "harness": "claude",
      "model": "fable",
      "effort": "high"
    }
  ]
}
```

All fields shown are required. Provide at least one worker profile with a
unique name. Remove the profiles for harnesses and models you don't use.

`lead` selects the settings for new teams. `profiles` defines the
choices offered to the lead when creating workers; descriptions
should explain which work suits each profile. Existing agents keep their
saved settings across restarts. Changes affect subsequently created agents,
and the available profiles are reread when tools and startup context are
requested.

`harness` names an installed [harness plugin](harness-support.md). Model
and effort values are interpreted by the selected harness. Claude Code
receives them as its model and effort; OpenCode uses effort as the model's
variant. Consult the harness for accepted values. Hypercube's exact schema
and validation are maintained in [config.go](../internal/harness/config.go).

## Files needed by worker worktrees

A new managed worktree starts at the lead's HEAD and initializes
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
