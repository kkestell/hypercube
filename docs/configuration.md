# Configuration

`config.json` lives at `~/.config/hypercube/config.json`. A non-empty
`XDG_CONFIG_HOME` selects `$XDG_CONFIG_HOME/hypercube/config.json` instead.
`config::load` reads and validates it. The orchestrator's harness, model, and
effort and every profile field shown below are required.

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
      "name": "deepseek",
      "description": "Bulk edits and mechanical refactors.",
      "harness": "opencode",
      "model": "openrouter/~deepseek/deepseek-flash-latest",
      "effort": "high"
    }
  ]
}
```

## Orchestrator

`orchestrator` sets the harness, model, and effort of the orchestrator agent.
When a workspace has no saved agents, `hypercube` creates the orchestrator
from these settings. Existing agents retain the harness, model, and effort
saved in the workspace state file when a session restarts.

## Profiles

`profiles` lists the combinations the orchestrator can give workers. Each has:

- `name`: unique within the file. `create_agent` takes it as `profile`.
- `description`: when to use the profile. The orchestrator reads it, together
  with the harness, model, and effort, in its SessionStart facts.
- `harness`, `model`, `effort`: the worker's settings, used exactly as given.

At least one profile is required. Worker creation reads the current file and
saves the chosen profile's settings on the new agent. The MCP tool list and
the orchestrator's SessionStart facts also read profiles from the current
file.

## Validation

Loading errors name the file; validation errors also identify the
orchestrator or profile involved. Loading fails when:

- the file is unreadable, is not valid JSON, or has values of the wrong type;
- `harness` is not `claude` or `opencode`, for the orchestrator or any profile;
- `model` or `effort` is empty;
- `profiles` is empty;
- a profile has no `name` or no `description`;
- two profiles share a `name`.

Names are compared exactly, including case. Required strings are checked for
zero length. Unknown fields are ignored.

## Harness settings

`harness` is `claude` or `opencode`.

Claude Code takes `model` as its `--model` and `effort` as its `--effort`.
OpenCode takes `model` as the model and `effort` as the model's variant.
Hypercube validates both as non-empty strings; the harness interprets the
supplied values.

Claude Code model names include `opus` and `haiku`. OpenCode takes the full
`provider/model` ID that `opencode models` lists, such as
`openrouter/~deepseek/deepseek-flash-latest`. OpenCode rejects a variant the
model does not list, and the pane shows the error. Since OpenCode otherwise
falls back to a recent model for an unavailable ID, the Hypercube plugin
refuses the first prompt of a session on another model.
