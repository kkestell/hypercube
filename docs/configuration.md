# Configuration

Create `~/.config/hypercube/config.json`, or
`$XDG_CONFIG_HOME/hypercube/config.json` when that variable is non-empty:

```json
{
  "profiles": [
    {
      "name": "opus",
      "role": "lead",
      "description": "Opus at high effort in Claude Code",
      "harness": "claude",
      "model": "opus",
      "effort": "medium"
    },
    {
      "name": "sol",
      "role": "lead",
      "description": "GPT Sol at high effort in Codex",
      "harness": "codex",
      "model": "gpt-6.1-sol",
      "effort": "high"
    },
    {
      "name": "opencode-deepseek-flash-max",
      "role": "general",
      "description": "DeepSeek V4.1 Flash at max effort for ordinary implementation, investigation, and review. Cheap and fast. Artificial Analysis: Terminal-Bench 4.0 26.8%; no FrontierCode 2.0 or cost per task listed.",
      "harness": "opencode",
      "model": "opencode-go/deepseek-v4.1-flash",
      "effort": "max"
    },
    {
      "name": "codex-gpt-luna-medium",
      "role": "general",
      "description": "GPT Luna 6 at medium effort only for trivial tasks such as lookups and quick checks. Weak at coding. Cheap and fast. Artificial Analysis: Terminal-Bench 4.0 2.5%, FrontierCode 2.0 12.8%, $0.04 per task.",
      "harness": "codex",
      "model": "gpt-6-luna",
      "effort": "medium"
    },
    {
      "name": "codex-gpt-sol-high",
      "role": "general",
      "description": "GPT Sol at high effort, only for code review and for implementation from a written plan. Its reviews include many speculative findings, so verify each one before acting on it. Never use it for design work: it over-engineers. Artificial Analysis: Terminal-Bench 4.0 51.5%, FrontierCode 2.0 36.7%, $0.59 per task.",
      "harness": "codex",
      "model": "gpt-6.1-sol",
      "effort": "high"
    },
    {
      "name": "codex-gpt-sol-xhigh",
      "role": "general",
      "description": "GPT Sol at extra high effort, only for hard code review and for hard implementation from a written plan. Its reviews include many speculative findings, so verify each one before acting on it. Never use it for design work: it over-engineers. Slower and costlier. Artificial Analysis: Terminal-Bench 4.0 54.0%, FrontierCode 2.0 38.5%, $0.76 per task.",
      "harness": "codex",
      "model": "gpt-6.1-sol",
      "effort": "xhigh"
    },
    {
      "name": "claude-haiku-xhigh",
      "role": "general",
      "description": "Haiku at extra high effort for small, well-defined tasks: lookups, simple edits, and quick checks. Artificial Analysis: Terminal-Bench 4.0 29.3%, FrontierCode 2.0 29.9%, $0.25 per task.",
      "harness": "claude",
      "model": "haiku",
      "effort": "xhigh"
    },
    {
      "name": "claude-sonnet-high",
      "role": "general",
      "description": "Sonnet at high effort for ordinary implementation, investigation, and review. Artificial Analysis: Terminal-Bench 4.0 43.9%, FrontierCode 2.0 25.7%, $1.64 per task.",
      "harness": "claude",
      "model": "sonnet",
      "effort": "high"
    },
    {
      "name": "claude-opus-medium",
      "role": "general",
      "description": "Opus at medium effort for hard implementation, debugging, and design work. Artificial Analysis: Terminal-Bench 4.0 52.5%, FrontierCode 2.0 33.1%, $2.45 per task.",
      "harness": "claude",
      "model": "opus",
      "effort": "medium"
    },
    {
      "name": "claude-opus-high",
      "role": "general",
      "description": "Opus at high effort for the hardest design and debugging work. Slower and costlier than claude-large. Artificial Analysis: Terminal-Bench 4.0 56.6%, FrontierCode 2.0 35.9%, $3.46 per task.",
      "harness": "claude",
      "model": "opus",
      "effort": "high"
    }
  ]
}
```

All fields shown are required, including each profile's role. Provide at least
one `lead` profile and one `general` profile, with unique names across all
profiles. Remove the profiles for harnesses and models you don't use.

The workspace's `+` menu lists the `lead` profiles by their descriptions, so a
lead's description is a short label.
Profiles with role `general` are offered to top-level sessions for children;
each description says which kind of work the profile suits and when to use it.
Existing agents keep their saved settings across restarts.
Changes affect subsequently created agents, and profiles are reread when the
session menu, tools, and startup context are requested.

`harness` names an installed [harness plugin](harness-support.md). Model
and effort values are interpreted by the selected harness. Claude Code
receives them as its model and effort; OpenCode uses effort as the model's
variant. Consult the harness for accepted values. Hypercube's exact schema
and validation are maintained in [config.go](../internal/harness/config.go).

## Files needed by child worktrees

A new managed worktree starts at the parent's HEAD and initializes
submodules. Uncommitted changes in the original checkout are absent.

To supply ignored local files, add a `.worktreeinclude` file at the checkout
root using Git ignore patterns, for example:

```gitignore
.env
node_modules/
```

Only paths also ignored by Git are copied. Copies are independent APFS
clones, so child sessions can change them without modifying the originals. They are
made when the worktree is created, not kept in sync. The implementation is
in [workspace.go](../internal/workspace/workspace.go).
