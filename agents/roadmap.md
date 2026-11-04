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
