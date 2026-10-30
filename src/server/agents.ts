import { PROTOCOL_VERSION, client, methods, ndJsonStream } from "@agentclientprotocol/sdk";
import type {
  AgentCapabilities,
  ClientConnection,
  McpServer,
  RequestPermissionRequest,
  RequestPermissionResponse,
  SessionConfigOption,
  SessionConfigSelectGroup,
  SessionConfigSelectOption,
  SessionNotification,
  ToolCallContent,
} from "@agentclientprotocol/sdk";
import { createPatch } from "diff";
import type { Agent, AgentInfo, AgentStatus, ConfigOption, QueuedPrompt, Session, ToolContent, TranscriptItem } from "../model";
import { loadConfig, type Profile, type Settings } from "./config";
import { environmentFor, type EnvironmentProcess } from "./environment";
import { ModelError } from "./errors";
import { saveItem } from "./store";

/**
 * Agent processes: each runs `opencode acp` in its session's clone, with
 * Hypercube as its ACP client. This owns the connection, the prompt queue and
 * the transcript, and keeps the agent's record in the model current.
 *
 * A process is started for an agent that has an ACP session already by
 * resuming that session. Transcripts outlive processes: they are kept for
 * every agent, running or not, and stored as they change.
 *
 * Runtime state lives on globalThis, so a `bun --hot` reload keeps the
 * processes and their transcripts.
 */

/**
 * ACP has no system prompt, so this goes in front of the lead's first prompt.
 * It names the tools as the agent sees them, `<server>_<tool>` in OpenCode.
 */
const leadPreamble = (repo: string, session: Session) => `You are the lead agent of a Hypercube session. Hypercube is a web UI where coding agents work on GitHub repos. You work in a clone of ${repo} in the session "${session.name}". Other leads may be working on ${repo} in their own sessions.

Hypercube's MCP server ("hypercube") gives you these tools:
- list_agents: the agents you can message (your workers and the other leads), with their ids and status.
- send_message(to, text): message one of them. \`to\` is an id or a name; use the id if names repeat.
- list_profiles: the profiles you can start workers on, each with a description of when to use it.
- start_worker(name, profile, dir, message?): start a worker on a profile in \`dir\`, a directory inside your clone: the clone itself or a git worktree you made (a relative path is relative to the clone). \`name\` must be unique in this session. The worker can't see your conversation, so \`message\` must hold everything it needs for its task. If a worker's status in list_agents is "exited", its \`error\` says why.
- stop_worker(id): stop and remove one of your workers.

A message from another agent arrives as a new prompt that contains "Message from <name> (id <id>, ...)". Answer it with send_message, because only the user sees your own replies. Your workers report to you the same way. Messages arrive while you work: your current tool call finishes, your turn is interrupted there, and the message comes as a new prompt that says so. What you had finished stays done; carry on from it with the message in mind. So after you start workers or send a message, don't wait or poll: do other work, or end your turn if there is none, and replies will reach you either way. Message another lead only when it helps, for example to avoid overlapping work. Don't reply to a message that needs no answer.

The user's request follows.`;

const workerPreamble = (repo: string, session: Session, agent: Agent, cwd: string, lead: Agent) => `You are "${agent.name}", a worker in a Hypercube session. Hypercube is a web UI where coding agents work on GitHub repos. You work in ${cwd}, part of a clone of ${repo} in the session "${session.name}". Your lead is "${lead.name}" (id ${lead.id}); it started you and gave you your task.

Hypercube's MCP server ("hypercube") gives you these tools:
- list_agents: the agents you can message, which is only your lead.
- send_message(to, text): message your lead. You can't message anyone else; anything for another agent goes through your lead.

A message from your lead arrives as a new prompt that contains "Message from <name> (id <id>, ...)". It can arrive while you work: your current tool call finishes, your turn is interrupted there, and the message comes as a new prompt that says so. What you had finished stays done; carry on from it with the message in mind. Only the user sees your own replies, so when you finish, or are blocked, report to your lead with send_message. Don't reply to a message that needs no answer.

Your task follows.`;

type Runtime = {
  agent: Agent;
  session: Session;
  /** Identifies the agent to Hypercube's MCP server. */
  token: string;
  /** Goes in front of the next prompt sent, once. */
  preamble?: string;
  /** Pushes the model after the agent's record changed. */
  changed: () => void;
  process?: EnvironmentProcess;
  connection?: ClientConnection;
  items: TranscriptItem[];
  /** What the panel shows besides the transcript. */
  info: AgentInfo;
  /** Answers the agent's open permission requests, by request id. */
  permissions: Map<string, (response: RequestPermissionResponse) => void>;
  /** While `session/load` replays the conversation: the transcript has it already. */
  replaying: boolean;
  /** Set when we kill the agent on purpose: whatever it does after that is ignored. */
  stopped: boolean;
  /** `session/cancel` is sent and the prompt it cancels hasn't returned yet. */
  cancelling: boolean;
  /** Set when the agent is stopped: the process is being stopped. The runtime is owned, and stays in `state.runtimes`, until this settles. */
  stopping?: Promise<void>;
  /** The agent's previous runtime while it is still stopping: replaced in `state.runtimes`, but still owned through this one. */
  previous?: Runtime;
};

type State = {
  /** The agents that have a process or had one since they were last stopped. A stopped agent stays until its process is gone. */
  runtimes: Map<string, Runtime>;
  /** Every agent's transcript. A runtime's `items` is the same array. */
  transcripts: Map<string, TranscriptItem[]>;
  /** Where agents reach Hypercube's MCP server. */
  mcpUrl: string;
  onItem: (agentId: string, item: TranscriptItem) => void;
  onInfo: (agentId: string, info: AgentInfo) => void;
};

const globals = globalThis as typeof globalThis & { __hypercubeAgents?: State };
const state: State = (globals.__hypercubeAgents ??= { runtimes: new Map(), transcripts: new Map(), mcpUrl: "", onItem: () => {}, onInfo: () => {} });

/** Where to send transcript and panel info changes. */
export function bindAgents(options: { mcpUrl: string; onItem: State["onItem"]; onInfo: State["onInfo"] }) {
  state.mcpUrl = options.mcpUrl;
  state.onItem = options.onItem;
  state.onInfo = options.onInfo;
}

/** Every transcript that has something in it. */
export function getTranscripts() {
  return [...state.transcripts].filter(([, items]) => items.length > 0).map(([agentId, items]) => ({ agentId, items }));
}

/** Loads the transcripts stored by an earlier server. */
export function restoreTranscripts(saved: Map<string, TranscriptItem[]>) {
  for (const [agentId, items] of saved) state.transcripts.set(agentId, items);
}

function transcriptOf(agentId: string) {
  let items = state.transcripts.get(agentId);
  if (!items) state.transcripts.set(agentId, (items = []));
  return items;
}

/** Drops the agents' transcripts from memory, for agents that are gone for good. The store drops its rows when the model is saved without them. */
export function forgetAgents(agentIds: string[]) {
  for (const agentId of agentIds) state.transcripts.delete(agentId);
}

/** Every agent's panel info that has something in it. */
export function getInfos() {
  return [...state.runtimes.values()].filter(rt => !rt.stopped && Object.keys(rt.info).length > 0).map(rt => ({ agentId: rt.agent.id, info: rt.info }));
}

/** The agent a Hypercube MCP token belongs to. A stopped agent's token no longer works. */
export function agentForToken(token: string | undefined) {
  if (!token) return undefined;
  for (const rt of state.runtimes.values()) if (rt.token === token && !rt.stopped) return rt.agent;
}

function runtime(agentId: string) {
  const rt = state.runtimes.get(agentId);
  if (!rt || rt.stopped) throw new ModelError("This agent is not running");
  return rt;
}

function setStatus(rt: Runtime, status: AgentStatus) {
  if (rt.stopped || rt.agent.status === status || rt.agent.status === "exited") return;
  rt.agent.status = status;
  rt.changed();
}

function upsert(rt: Runtime, item: TranscriptItem) {
  if (rt.stopped) return;
  const at = rt.items.findLastIndex(existing => existing.id === item.id);
  if (at === -1) rt.items.push(item);
  else rt.items[at] = item;
  saveItem(rt.agent.id, item);
  state.onItem(rt.agent.id, item);
}

/** Replaces the given fields of the panel info and sends the whole of it. */
function setInfo(rt: Runtime, patch: AgentInfo) {
  if (rt.stopped) return;
  rt.info = { ...rt.info, ...patch };
  state.onInfo(rt.agent.id, rt.info);
}

function find(rt: Runtime, id: string) {
  return rt.items.findLast(item => item.id === id);
}

function notice(rt: Runtime, text: string) {
  upsert(rt, { id: crypto.randomUUID(), kind: "notice", text });
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}

/** The settings an agent runs on: the config's `lead` for a lead, a profile (as given, or by name from the config) for a worker. */
async function settingsFor(agent: Agent, profile: Profile | undefined) {
  if (profile) return profile;
  const config = await loadConfig();
  if (agent.role === "lead") return config.lead;
  const found = config.profiles.find(candidate => candidate.name === agent.profile);
  if (!found) throw new ModelError(`Profile "${agent.profile}" is not in the config any more`);
  return found;
}

/**
 * Starts the agent's process and ACP session. An agent that has a session
 * resumes it, and gets a new one with a notice if it can't. Failures leave the
 * agent exited with the error. Registers the agent's runtime first, so it is
 * owned from here on, then waits for the previous process (if any) to be gone
 * and for the session's environment to be up. `lead` is the session's lead (the
 * agent itself, for a lead); `profile` is the worker's, when it was just chosen.
 */
export async function startAgent(agent: Agent, session: Session, repo: string, changed: () => void, { lead, profile }: { lead: Agent; profile?: Profile }) {
  const items = transcriptOf(agent.id);
  const preamble = agent.role === "worker" ? workerPreamble(repo, session, agent, agent.cwd, lead) : leadPreamble(repo, session);
  const old = state.runtimes.get(agent.id);
  const rt: Runtime = {
    agent,
    session,
    token: crypto.randomUUID(),
    // An agent that was prompted before has it already.
    preamble: items.some(item => item.kind === "user") ? undefined : preamble,
    changed,
    items,
    info: {},
    permissions: new Map(),
    replaying: false,
    stopped: false,
    cancelling: false,
    previous: old,
  };
  state.runtimes.set(agent.id, rt);
  // A new process starts with no panel info; the browser still has the old one.
  state.onInfo(agent.id, {});
  try {
    if (old) await stopRuntime(old);
    rt.previous = undefined;
    // Whatever the previous process was doing is lost.
    abandonTurn(rt);
    // Read now, so an edit to the config applies to the next agent and a bad file stops this one with its error.
    const settings = await settingsFor(agent, profile);
    // A Docker container may be stopped (Docker restarted) or gone; a prompt sent meanwhile is queued, since the agent is `starting`.
    const environment = environmentFor(session);
    await environment.ensureRunning();
    if (rt.stopped) return;
    const proc = environment.spawn(["opencode", "acp"], agent.cwd);
    rt.process = proc;
    void proc.exited.then(code => exited(rt, `opencode exited with code ${code}`));
    void logStderr(agent, proc.stderr);

    const stdin = proc.stdin;
    const stream = ndJsonStream(
      new WritableStream<Uint8Array>({
        async write(chunk) {
          stdin.write(chunk);
          await stdin.flush();
        },
      }),
      proc.stdout,
    );
    const connection = client({ name: "hypercube" })
      .onRequest(methods.client.session.requestPermission, ({ params }) => requestPermission(rt, params))
      .onNotification(methods.client.session.update, ({ params }) => applyUpdate(rt, params))
      .connect(stream);
    rt.connection = connection;

    // No fs or terminal capabilities: the agent uses its own tools in its own environment.
    const { agentCapabilities } = await connection.agent.request(methods.agent.initialize, {
      protocolVersion: PROTOCOL_VERSION,
      clientCapabilities: {},
      clientInfo: { name: "hypercube", version: "0.0.0" },
    });
    // A fresh token each time: the old process is gone and its token with it.
    const mcpServers: McpServer[] = [{ type: "http", name: "hypercube", url: environment.hostUrl(state.mcpUrl), headers: [{ name: "Authorization", value: `Bearer ${rt.token}` }] }];
    const resumed = agent.acpSessionId ? await resumeSession(rt, agentCapabilities, { cwd: agent.cwd, mcpServers }) : undefined;
    if (rt.stopped) return;
    let configOptions: SessionConfigOption[];
    if (resumed) {
      configOptions = resumed.configOptions ?? [];
    } else {
      const created = await connection.agent.request(methods.agent.session.new, { cwd: agent.cwd, mcpServers });
      if (rt.stopped) return;
      if (agent.acpSessionId) {
        notice(rt, "The earlier conversation could not be resumed; this is a new one, and the agent does not remember it.");
        rt.preamble = preamble;
      }
      agent.acpSessionId = created.sessionId;
      configOptions = created.configOptions ?? [];
    }
    const options = await applySettings(rt, toConfigOptions(configOptions), settings, agent.role === "worker" ? `Profile "${agent.profile}"` : "The lead settings");
    if (rt.stopped) return;
    setInfo(rt, { configOptions: options });
    setStatus(rt, "idle");
    sendNext(rt);
  } catch (error) {
    void rt.process?.stop();
    // A ModelError is the config's fault and already says so.
    exited(rt, error instanceof ModelError ? error.message : `Could not start opencode: ${errorMessage(error)}`);
  }
}

/**
 * Brings the agent's recorded ACP session back in the new process: `session/resume`,
 * else `session/load`. Undefined if neither works, and the caller starts a new session.
 */
async function resumeSession(rt: Runtime, capabilities: AgentCapabilities | undefined, params: { cwd: string; mcpServers: McpServer[] }) {
  const sessionId = rt.agent.acpSessionId!;
  const failed = (method: string, error: unknown) => console.error(`[${rt.agent.name} ${rt.agent.id.slice(0, 8)}] ${method} failed: ${errorMessage(error)}`);
  if (capabilities?.sessionCapabilities?.resume) {
    try {
      return await rt.connection!.agent.request(methods.agent.session.resume, { sessionId, ...params });
    } catch (error) {
      failed("session/resume", error);
    }
  }
  if (capabilities?.loadSession) {
    // Load replays the conversation as updates. We have it already.
    rt.replaying = true;
    try {
      return await rt.connection!.agent.request(methods.agent.session.load, { sessionId, ...params });
    } catch (error) {
      failed("session/load", error);
    } finally {
      rt.replaying = false;
    }
  }
}

/**
 * Sets the agent's model, then its effort (the model decides which efforts
 * exist), to what `settings` say. A value the agent doesn't offer or can't set
 * stops it: `who` names the profile or lead setting at fault.
 */
async function applySettings(rt: Runtime, options: ConfigOption[], settings: Settings, who: string) {
  for (const [category, value] of [["model", settings.model], ["thought_level", settings.effort]] as const) {
    const option = options.find(option => option.category === category);
    if (!option) throw new ModelError(`${who}: OpenCode has no ${category} option to set to "${value}"`);
    if (!option.options.some(choice => choice.value === value)) {
      const offered = option.options.map(choice => choice.value);
      const list = offered.length > 20 ? `${offered.slice(0, 20).join(", ")} and ${offered.length - 20} more` : offered.join(", ");
      throw new ModelError(`${who}: OpenCode doesn't offer ${category} "${value}". It offers: ${list}`);
    }
    if (option.currentValue === value) continue;
    try {
      options = toConfigOptions(await requestConfigOption(rt, option.id, value));
    } catch (error) {
      throw new ModelError(`${who}: could not set ${category} to "${value}": ${errorMessage(error)}`);
    }
  }
  return options;
}

async function requestConfigOption(rt: Runtime, configId: string, value: string) {
  const { configOptions } = await rt.connection!.agent.request(methods.agent.session.setConfigOption, {
    sessionId: rt.agent.acpSessionId!,
    configId,
    value,
  });
  return configOptions;
}

/** Keeps the select options, with grouped values flattened. We don't advertise boolean options. */
function toConfigOptions(options: SessionConfigOption[]): ConfigOption[] {
  return options.flatMap(option => {
    if (option.type !== "select") return [];
    const choices = (option.options as (SessionConfigSelectOption | SessionConfigSelectGroup)[]).flatMap(choice => ("group" in choice ? choice.options : [choice]));
    return {
      id: option.id,
      name: option.name,
      category: option.category ?? undefined,
      currentValue: option.currentValue,
      options: choices.map(({ value, name, description }) => ({ value, name, description: description ?? undefined })),
    };
  });
}

async function logStderr(agent: Agent, stderr: ReadableStream<Uint8Array>) {
  const decoder = new TextDecoder();
  for await (const chunk of stderr) {
    const text = decoder.decode(chunk, { stream: true }).trim();
    if (text) console.error(`[${agent.name} ${agent.id.slice(0, 8)}] ${text}`);
  }
}

/** The process is gone or never started. */
function exited(rt: Runtime, error: string) {
  if (rt.stopped || rt.agent.status === "exited") return;
  abandonTurn(rt);
  rt.connection?.close();
  rt.agent.queue = [];
  rt.agent.steer = undefined;
  rt.agent.status = "exited";
  rt.agent.error = error;
  notice(rt, error);
  rt.changed();
}

/** Folds an ACP session update into the transcript and the panel info. Updates we don't render are dropped. */
function applyUpdate(rt: Runtime, { update }: SessionNotification) {
  if (rt.replaying) return;
  switch (update.sessionUpdate) {
    case "agent_message_chunk":
    case "agent_thought_chunk": {
      if (update.content.type !== "text") return;
      const kind = update.sessionUpdate === "agent_message_chunk" ? "agent" : "thought";
      // Chunks of one message share a messageId; without one, they run on from the previous item.
      const last = rt.items.at(-1);
      const id = update.messageId ? `${kind}-${update.messageId}` : last?.kind === kind ? last.id : crypto.randomUUID();
      const existing = find(rt, id);
      const before = existing?.kind === kind ? existing.text : "";
      upsert(rt, { id, kind, text: before + update.content.text });
      break;
    }
    case "tool_call":
    case "tool_call_update": {
      const id = `tool-${update.toolCallId}`;
      const existing = find(rt, id);
      const before = existing?.kind === "tool" ? existing : undefined;
      upsert(rt, {
        id,
        kind: "tool",
        title: update.title ?? before?.title ?? "tool",
        toolKind: update.kind ?? before?.toolKind ?? "other",
        status: update.status ?? before?.status ?? "pending",
        // An update that carries content or locations replaces the earlier ones.
        content: update.content ? toolContent(update.content) : (before?.content ?? []),
        locations: update.locations ? update.locations.map(({ path, line }) => ({ path, line: line ?? undefined })) : (before?.locations ?? []),
      });
      advanceSteer(rt);
      break;
    }
    case "plan":
      setInfo(rt, { plan: update.entries.map(({ content, status }) => ({ content, status })) });
      break;
    case "usage_update":
      setInfo(rt, { usage: { used: update.used, size: update.size, cost: update.cost ? { amount: update.cost.amount, currency: update.cost.currency } : undefined } });
      break;
    case "available_commands_update":
      setInfo(rt, { commands: update.availableCommands.map(({ name, description, input }) => ({ name, description, hint: input?.hint })) });
      break;
    case "config_option_update":
      setInfo(rt, { configOptions: toConfigOptions(update.configOptions) });
      break;
    case "session_info_update":
      // The lead's title names the session until the user would have no reason to keep "Untitled N".
      if (rt.agent.role === "lead" && update.title && /^Untitled \d+$/.test(rt.session.name)) {
        rt.session.name = update.title;
        rt.changed();
      }
      break;
  }
}

/** Text and diffs. Images, resources and terminals aren't shown. */
function toolContent(content: ToolCallContent[]): ToolContent[] {
  return content.flatMap((block): ToolContent[] => {
    if (block.type === "diff") {
      // Drop the patch header (index line, rule, and the two file names): the panel shows the path itself.
      const patch = createPatch(block.path, block.oldText ?? "", block.newText, undefined, undefined, { context: 3 }).split("\n").slice(4).join("\n");
      return [{ type: "diff", path: block.path, patch }];
    }
    if (block.type === "content" && block.content.type === "text") return [{ type: "text", text: block.content.text }];
    return [];
  });
}

const cancelled: RequestPermissionResponse = { outcome: { outcome: "cancelled" } };

async function requestPermission(rt: Runtime, params: RequestPermissionRequest) {
  // A turn that is being cut short has nobody to answer, and a request that reopened `waiting` would hold the steer until someone did.
  if (rt.cancelling || rt.agent.steer) return cancelled;
  const requestId = crypto.randomUUID();
  upsert(rt, {
    id: `permission-${requestId}`,
    kind: "permission",
    requestId,
    title: params.toolCall.title ?? "Permission requested",
    options: params.options.map(({ optionId, name, kind }) => ({ optionId, name, kind })),
  });
  setStatus(rt, "waiting");
  return new Promise<RequestPermissionResponse>(resolve => rt.permissions.set(requestId, resolve));
}

/**
 * Answers every open permission request as cancelled, as ACP requires when a
 * turn is cancelled. Requests left open in the transcript by a process that
 * died are closed too.
 */
function closePermissions(rt: Runtime) {
  for (const resolve of rt.permissions.values()) resolve(cancelled);
  rt.permissions.clear();
  for (const item of rt.items) if (item.kind === "permission" && !item.answer) upsert(rt, { ...item, answer: "cancelled" });
}

/** The agent may leave tool calls unfinished when it stops. */
function cancelUnfinishedTools(rt: Runtime) {
  for (const item of rt.items) if (item.kind === "tool" && (item.status === "pending" || item.status === "in_progress")) upsert(rt, { ...item, status: "cancelled" });
}

/** The turn in progress, if any, ends with its process. */
function abandonTurn(rt: Runtime) {
  closePermissions(rt);
  cancelUnfinishedTools(rt);
}

/**
 * Delivers a prompt. An idle agent gets it now. A busy agent is steered, unless
 * `mode` is "queue": the prompt interrupts its turn (see `steer`) instead of
 * waiting behind it. `from` is the sending agent, when it isn't the user.
 */
export function promptAgent(agentId: string, text: string, from?: { id: string; label: string }, mode: "steer" | "queue" = "steer") {
  const rt = runtime(agentId);
  if (!text.trim()) throw new ModelError("Enter a prompt");
  if (rt.agent.status === "exited") throw new ModelError("This agent has exited");
  const prompt: QueuedPrompt = { text, from: from?.label, fromId: from?.id, sentAt: Date.now() };
  if (mode === "steer" && (rt.agent.status === "busy" || rt.agent.status === "waiting")) return steer(rt, prompt);
  rt.agent.queue.push(mode === "steer" ? { ...prompt, steer: true } : prompt);
  rt.changed();
  sendNext(rt);
}

/**
 * Redirects a busy agent. OpenCode accepts one prompt at a time, so the turn is
 * cancelled and the steer becomes the next prompt, ahead of the queue. A tool
 * call that is running is let finish first, so the agent knows what ran; if
 * none is, the turn is cancelled at once. Steers that arrive before the turn
 * stops go out as one prompt.
 */
function steer(rt: Runtime, prompt: QueuedPrompt) {
  (rt.agent.steer ??= { prompts: [] }).prompts.push(prompt);
  // Answered now: nobody should have to answer a request in a turn that is being cut short.
  closePermissions(rt);
  setStatus(rt, "busy");
  advanceSteer(rt);
  rt.changed();
}

/** Cancels the turn for a pending steer once no tool call is running; until then, says which one it waits for. */
function advanceSteer(rt: Runtime) {
  const pending = rt.agent.steer;
  if (!pending || rt.cancelling) return;
  const running = rt.items.find(item => item.kind === "tool" && item.status === "in_progress");
  if (!running) return cancelTurn(rt);
  const title = (running as Extract<TranscriptItem, { kind: "tool" }>).title;
  if (pending.waitingFor !== title) {
    pending.waitingFor = title;
    rt.changed();
  }
}

/** Sends `session/cancel`, which makes the prompt in flight return `cancelled`. */
function cancelTurn(rt: Runtime) {
  rt.cancelling = true;
  if (rt.agent.steer) rt.agent.steer.waitingFor = undefined;
  closePermissions(rt);
  setStatus(rt, "busy");
  void rt.connection!.agent.notify(methods.agent.session.cancel, { sessionId: rt.agent.acpSessionId! });
  rt.changed();
}

/** Cancels the turn now, without waiting for the running tool call; the pending steer follows. */
export function interruptAgent(agentId: string) {
  const rt = runtime(agentId);
  // The steer may have gone out since the user looked.
  if (rt.agent.steer && !rt.cancelling) cancelTurn(rt);
}

/** Sends the next queued prompt if the agent is ready for one. */
function sendNext(rt: Runtime) {
  if (rt.agent.status !== "idle") return;
  const next = rt.agent.queue.shift();
  if (next) void runTurn(rt, [next]);
}

function ago(ms: number) {
  const seconds = Math.round(ms / 1000);
  if (seconds < 2) return "just now";
  if (seconds < 60) return `${seconds} seconds ago`;
  const [count, unit] = seconds < 3600 ? [Math.round(seconds / 60), "minute"] : [Math.round(seconds / 3600), "hour"];
  return `${count} ${unit}${count === 1 ? "" : "s"} ago`;
}

/**
 * What the agent is told about a message that isn't plain user text. The
 * transcript shows what the sender wrote. Agents tend to answer a message in
 * their own transcript, which the sender never sees, so a message from an agent
 * says how to reply.
 */
function describe({ text, from, fromId, sentAt }: QueuedPrompt) {
  const sent = `sent ${ago(Date.now() - sentAt)}`;
  if (!from) return `Message from the user (${sent}):\n\n${text}`;
  return `Message from ${from} (id ${fromId}, ${sent}). The sender can't see your transcript; to answer, call send_message with to "${fromId}".\n\n${text}`;
}

const steerNotes = {
  interrupted:
    "Sent while you were working. Your previous turn was interrupted after its last finished step: the tool calls that finished did run, and what you were about to do next has not happened. Carry on with that in mind.",
  ended: "Sent while you were working; your previous turn had already ended by the time it arrived.",
};

async function runTurn(rt: Runtime, prompts: QueuedPrompt[], steered?: { interrupted: boolean }) {
  setStatus(rt, "busy");
  for (const { text, from } of prompts) upsert(rt, { id: crypto.randomUUID(), kind: "user", text, from, steered: steered ? true : undefined });
  const [first] = prompts;
  const body = steered ? `${steerNotes[steered.interrupted ? "interrupted" : "ended"]}\n\n${prompts.map(describe).join("\n\n---\n\n")}` : first!.from ? describe(first!) : first!.text;
  const parts = [rt.preamble, body];
  rt.preamble = undefined;
  let interrupted = false;
  try {
    const turn = rt.connection!.agent.request(methods.agent.session.prompt, {
      sessionId: rt.agent.acpSessionId!,
      prompt: [{ type: "text", text: parts.filter(Boolean).join("\n\n") }],
    });
    // Steers that were queued because no turn was running interrupt this one, now that there is a turn to cancel.
    const queuedSteers = rt.agent.queue.filter(prompt => prompt.steer);
    if (queuedSteers.length) {
      rt.agent.queue = rt.agent.queue.filter(prompt => !prompt.steer);
      for (const prompt of queuedSteers) steer(rt, prompt);
    }
    const { stopReason } = await turn;
    if (stopReason === "cancelled") {
      interrupted = true;
    } else if (stopReason !== "end_turn") {
      notice(rt, `The turn ended: ${stopReason}`);
    }
  } catch (error) {
    if (rt.agent.status !== "exited") notice(rt, `The prompt failed: ${errorMessage(error)}`);
  }
  // A tool call left unfinished by any turn end would otherwise hold every later steer waiting for it.
  cancelUnfinishedTools(rt);
  rt.cancelling = false;
  closePermissions(rt);
  setStatus(rt, "idle");
  const pending = rt.agent.steer;
  rt.agent.steer = undefined;
  // The steer goes before anything queued.
  if (pending && rt.agent.status === "idle") void runTurn(rt, pending.prompts, { interrupted });
  else sendNext(rt);
}

/** Stops the current turn. Queued prompts and a pending steer are kept and go out next: they may be messages from other agents. */
export function cancelAgent(agentId: string) {
  const rt = runtime(agentId);
  if ((rt.agent.status === "busy" || rt.agent.status === "waiting") && !rt.cancelling) cancelTurn(rt);
  rt.changed();
}

/** Changes one of the agent's session config options (model, effort, mode). */
export async function setConfigOption(agentId: string, configId: string, value: string) {
  const rt = runtime(agentId);
  if (rt.agent.status === "exited") throw new ModelError("This agent has exited");
  const option = rt.info.configOptions?.find(option => option.id === configId);
  if (!option) throw new ModelError("No such option");
  if (!option.options.some(choice => choice.value === value)) throw new ModelError(`Not one of ${option.name}'s values`);
  try {
    setInfo(rt, { configOptions: toConfigOptions(await requestConfigOption(rt, configId, value)) });
  } catch (error) {
    throw new ModelError(`Could not set ${option.name}: ${errorMessage(error)}`);
  }
}

export function answerPermission(agentId: string, requestId: string, optionId: string) {
  const rt = runtime(agentId);
  const resolve = rt.permissions.get(requestId);
  const item = find(rt, `permission-${requestId}`);
  if (!resolve || item?.kind !== "permission") throw new ModelError("That permission request is no longer open");
  if (!item.options.some(option => option.optionId === optionId)) throw new ModelError("Not one of the request's options");
  rt.permissions.delete(requestId);
  resolve({ outcome: { outcome: "selected", optionId } });
  upsert(rt, { ...item, answer: optionId });
  if (rt.permissions.size === 0) setStatus(rt, "busy");
}

/**
 * Stops the agent's process (and what it owns) and waits for it to be gone. The
 * runtime is ours until then, so a shutdown meanwhile waits for it too.
 */
function stopRuntime(rt: Runtime): Promise<void> {
  if (!rt.stopping) {
    abandonTurn(rt);
    rt.stopped = true;
    rt.connection?.close();
    const previous = rt.previous ? stopRuntime(rt.previous) : undefined;
    rt.stopping = Promise.all([rt.process?.stop(), previous]).then(() => {}).finally(() => {
      if (state.runtimes.get(rt.agent.id) === rt) state.runtimes.delete(rt.agent.id);
    });
  }
  return rt.stopping;
}

/** Stops the agents' processes and waits for them to go. */
export async function stopAgents(agentIds: string[]) {
  await Promise.all(agentIds.flatMap(id => state.runtimes.get(id) ?? []).map(stopRuntime));
}

/** Stops every agent, including ones already being stopped, and waits for them to go. For server shutdown. */
export function stopAllAgents() {
  return stopAgents([...state.runtimes.keys()]);
}

/** Kills every agent's processes without waiting, for when the server process is exiting. */
export function stopAllAgentsNow() {
  for (const rt of state.runtimes.values()) {
    for (let current: Runtime | undefined = rt; current; current = current.previous) current.process?.stopNow();
  }
}
