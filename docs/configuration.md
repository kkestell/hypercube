# Configuration

`config.json` lives at `~/.config/hypercube/config.json`. `XDG_CONFIG_HOME`
overrides the parent directory. `config::load` reads and validates it; every
value must be set.

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
      "harness": "pi",
      "model": "openrouter/~deepseek/deepseek-flash-latest",
      "effort": "medium"
    }
  ]
}
```

## Orchestrator

`orchestrator` sets the harness, model, and effort of the orchestrator agent.
The first `hypercube` run in a workspace creates the orchestrator from it.

## Profiles

`profiles` lists the combinations the orchestrator can give workers. Each has:

- `name`: unique within the file. `create_agent` takes it as `profile`.
- `description`: when to use the profile. The orchestrator reads it, together
  with the harness, model, and effort, in its SessionStart facts.
- `harness`, `model`, `effort`: the worker's settings, used exactly as given.

At least one profile is required.

## Validation

Loading fails, naming the file and the offending entry, when:

- the file is missing or is not valid JSON;
- `harness` is not `claude` or `pi`, for the orchestrator or any profile;
- `model` or `effort` is empty;
- `profiles` is empty;
- a profile has no `name` or no `description`;
- two profiles share a `name`.

Unknown fields are ignored.

## Harness settings

`harness` is `claude` or `pi`.

`effort` is passed through as Claude Code's `--effort` and Pi's `--thinking`.

Claude Code models are the names `--model` accepts, such as `opus` or `haiku`.
Pi models use the full `provider/id`, such as `openai/gpt-6.1-sol` or
`openrouter/deepseek/deepseek-v4.1-flash-1017`. Pi can fuzzy-match a partial ID
to another model and passes an unknown ID to the provider, which rejects it in
the pane.

An agent saved in a state file with any other harness fails to launch, and the
host reports the error.
