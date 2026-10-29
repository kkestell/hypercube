import { existsSync, mkdirSync, readdirSync, renameSync, writeFileSync } from "node:fs";
import { homedir } from "node:os";
import { dirname, join } from "node:path";
import type { Agent } from "../model";

/**
 * How each harness is started and resumed, and the hook and plugin files that
 * let a running harness tell the server which conversation it is on.
 *
 * The files are the same for every server instance; what differs per process
 * (the server's URL and the agent's token) is passed in the environment.
 */

export const harnessDir = () => process.env.HYPERCUBE_HARNESS_DIR ?? join(homedir(), ".cache/hypercube/harness");

const claudeSettingsPath = () => join(harnessDir(), "claude-settings.json");
const opencodePluginDir = () => join(harnessDir(), "opencode-plugin");

const CLAUDE_HOOK = `#!/bin/sh
# Reports Claude Code's SessionStart to Hypercube. Never fails Claude.
[ -n "$HYPERCUBE_TOKEN" ] && [ -n "$HYPERCUBE_URL" ] || exit 0
curl -s -m 2 -H "Authorization: Bearer $HYPERCUBE_TOKEN" -H 'Content-Type: application/json' \\
  --data-binary @- "$HYPERCUBE_URL/api/agent/claude-session-start" >/dev/null 2>&1
exit 0
`;

const OPENCODE_PLUGIN_PACKAGE = JSON.stringify(
  { name: "hypercube-session-tracker", type: "module", exports: { "./tui": "./tui.ts" } },
  null,
  2,
);

const OPENCODE_PLUGIN = `// Reports the TUI's current route to Hypercube, in order, one request at a time.
import { Plugin } from "@opencode/plugin/tui";
import { createEffect } from "solid-js";

const url = process.env.HYPERCUBE_URL;
const token = process.env.HYPERCUBE_TOKEN;
let seq = 0;
let chain = Promise.resolve();

function report(route) {
  if (!url || !token) return;
  const body = JSON.stringify({ seq: ++seq, route });
  chain = chain.then(() =>
    fetch(url + "/api/agent/opencode-route", {
      method: "POST",
      headers: { authorization: "Bearer " + token, "content-type": "application/json" },
      body,
    }).then(
      () => {},
      () => {},
    ),
  );
}

export default Plugin.define({
  id: "hypercube-session-tracker",
  setup(ctx) {
    return ctx.ui.slot({
      append: "app",
      render: () => {
        createEffect(() => {
          const route = ctx.ui.router.current();
          report(route?.type === "session" ? { type: "session", sessionID: route.sessionID } : { type: route?.type ?? "unknown" });
        });
        return null;
      },
    });
  },
});
`;

/** Writes a file so that a reader sees either the old or the new content, never part of it. */
function writeAtomic(path: string, content: string, mode = 0o644) {
  mkdirSync(dirname(path), { recursive: true });
  const temp = `${path}.${process.pid}.${crypto.randomUUID()}.tmp`;
  writeFileSync(temp, content, { mode });
  renameSync(temp, path);
}

/** Writes the hook and plugin files. Called at startup and on every reload. */
export function writeHarnessFiles() {
  const hook = join(harnessDir(), "claude-hook.sh");
  writeAtomic(hook, CLAUDE_HOOK, 0o755);
  writeAtomic(
    claudeSettingsPath(),
    JSON.stringify({ hooks: { SessionStart: [{ hooks: [{ type: "command", command: hook, timeout: 5 }] }] } }, null, 2),
  );
  writeAtomic(join(opencodePluginDir(), "package.json"), OPENCODE_PLUGIN_PACKAGE);
  writeAtomic(join(opencodePluginDir(), "tui.ts"), OPENCODE_PLUGIN);
}

export function newConversationId(harness: Agent["harness"]) {
  return harness === "claude" ? crypto.randomUUID() : `ses_hypercube_${crypto.randomUUID().replace(/-/g, "")}`;
}

/**
 * Whether Claude Code has a transcript for the conversation, so it can be
 * resumed. A conversation has none until something is recorded in it, and
 * resuming one without a transcript fails.
 */
function claudeTranscriptExists(conversationId: string, transcriptPath: string | null) {
  if (transcriptPath) return existsSync(transcriptPath);
  // Hooks may be disabled, so the path was never reported. Transcripts live in a directory per project.
  const projects = join(process.env.CLAUDE_CONFIG_DIR ?? join(homedir(), ".claude"), "projects");
  try {
    return readdirSync(projects).some(project => existsSync(join(projects, project, `${conversationId}.jsonl`)));
  } catch {
    return false;
  }
}

/** The command and extra environment that start or resume an agent's conversation. */
export function harnessCommand(agent: Agent, transcriptPath: string | null): { command: string[]; env: Record<string, string> } {
  if (agent.harness === "claude") {
    const bin = process.env.HYPERCUBE_CLAUDE_BIN ?? "claude";
    const resume = claudeTranscriptExists(agent.conversationId, transcriptPath);
    return {
      command: [bin, resume ? "--resume" : "--session-id", agent.conversationId, "--settings", claudeSettingsPath()],
      env: {},
    };
  }
  const bin = process.env.HYPERCUBE_OPENCODE_BIN ?? "opencode";
  return {
    // --standalone runs a private server in the terminal's session, so stopping the terminal stops its work.
    command: [bin, "--standalone", "--session", agent.conversationId],
    env: { OPENCODE_CLI_CONFIG_CONTENT: JSON.stringify({ plugins: [opencodePluginDir()] }) },
  };
}
