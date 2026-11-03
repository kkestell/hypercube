# Roadmap

## Codex async questions use interactive controls

Codex's `request_user_input_async` sends assistant message items with
`questions` and `delivery: "async"`. The Codex adapter ignores those fields,
so questions appear as ordinary text without selectable answers. Its existing
`item/tool/requestUserInput` handler covers blocking questions only.

Translate async question metadata into interactive attention items, preserving
question titles and options, and support submitting the user's answers to the
agent. Preserve the existing blocking-question support.

## Codex MCP tool approvals offer Always allow

Codex's MCP tool approvals arrive as `mcpServer/elicitation/request` without
the tool's name or the elicitation's own ID, so the adapter has no key to save
and offers no Always allow for them.

Offer Always allow once Codex includes a stable tool identifier in the request.

## open_file opens files anywhere on the filesystem

The `open_file` MCP tool reads through `os.OpenRoot` on the calling session's
directory, so it refuses absolute paths outside that directory, `..` paths
that escape it, and symlinks that point outside it. `~` is not expanded.

Allow `open_file` to open any file the user can read, including absolute paths
outside the workspace and `~`-prefixed paths, so an agent can show the user a
file from anywhere on the machine.
