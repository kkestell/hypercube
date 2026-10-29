/**
 * Stands in for Claude Code or OpenCode in tests. Logs how it was started,
 * reports to Hypercube the way the real harness's hook or plugin would, and
 * takes commands on stdin:
 *   record          create the conversation's transcript (claude)
 *   /clear          switch to a new conversation and report it (claude)
 *   route <seq> <id|home>  report a TUI route (opencode)
 *   child           start a background process in this session
 *   exit <code>     exit
 * If $FAKE_DIR/crash exists, it exits 1 right after logging.
 */
import { appendFileSync, existsSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";

const [harness, ...args] = process.argv.slice(2);
const dir = process.env.FAKE_DIR!;
const flag = (name: string) => {
  const index = args.indexOf(name);
  return index >= 0 ? args[index + 1] : undefined;
};

appendFileSync(
  join(dir, "log.jsonl"),
  JSON.stringify({ harness, args, pid: process.pid, cwd: process.cwd(), token: process.env.HYPERCUBE_TOKEN, url: process.env.HYPERCUBE_URL }) + "\n",
);
console.log(`fake ${harness} ${args.join(" ")}`);
if (existsSync(join(dir, "crash"))) process.exit(1);

let conversation = harness === "claude" ? (flag("--resume") ?? flag("--session-id"))! : flag("--session")!;
const transcript = (id: string) => join(dir, `${id}.jsonl`);

/** Runs the SessionStart hook from --settings, as Claude Code does. */
function claudeHook(source: string) {
  const settings = JSON.parse(readFileSync(flag("--settings")!, "utf8"));
  const command: string = settings.hooks.SessionStart[0].hooks[0].command;
  const input = JSON.stringify({ session_id: conversation, transcript_path: transcript(conversation), cwd: process.cwd(), hook_event_name: "SessionStart", source });
  Bun.spawnSync(["/bin/sh", "-c", command], { stdin: new TextEncoder().encode(input) });
}

async function route(seq: number, route: unknown) {
  await fetch(`${process.env.HYPERCUBE_URL}/api/agent/opencode-route`, {
    method: "POST",
    headers: { authorization: `Bearer ${process.env.HYPERCUBE_TOKEN}`, "content-type": "application/json" },
    body: JSON.stringify({ seq, route }),
  }).catch(() => {});
}

if (harness === "claude") claudeHook(flag("--resume") ? "resume" : "startup");

for await (const line of console) {
  const [command, ...rest] = line.trim().split(/\s+/);
  if (command === "record") writeFileSync(transcript(conversation), "{}\n");
  else if (command === "/clear") {
    conversation = crypto.randomUUID();
    claudeHook("clear");
  } else if (command === "route") await route(Number(rest[0]), rest[1] === "home" ? { type: "home" } : { type: "session", sessionID: rest[1] });
  else if (command === "child") Bun.spawn(["sleep", "7801"], { stdio: ["ignore", "ignore", "ignore"] }).unref();
  else if (command === "exit") process.exit(Number(rest[0] ?? 0));
  console.log(`ok ${command}`);
}
