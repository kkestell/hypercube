import { realpathSync, statSync } from "node:fs";
import { homedir } from "node:os";
import { basename, isAbsolute } from "node:path";
import { HARNESS_LABELS, emptyModel, type Agent, type Harness, type Model, type Session, type Workspace } from "../model";
import { harnessCommand, newConversationId, writeHarnessFiles } from "./harness";
import { closeTerminal, createTerminal, setOwnerHooks, type TerminalEntry } from "./terminals";

/**
 * The workspace/session/agent model and the lifecycle of the processes behind
 * it. The server owns it; browsers get it pushed and only ask for changes.
 *
 * Every transition is one synchronous handler. Anything slow (a close
 * finishing, a process ending, a retry timer, a hook request) comes back as an
 * event, and the event looks up the terminal's owner and the agent's state
 * when it runs, so an event that was overtaken by another just finds nothing
 * to do. State lives on globalThis and timers and completions call through
 * `state.ops`, so a `bun --hot` reload keeps the model and uses the new code.
 */

/** A lead that exits sooner than this after starting counts towards the crash loop. */
const QUICK_EXIT_MS = 10_000;
const MAX_QUICK_EXITS = 3;
const RETRY_BASE_MS = 500;
const RETRY_MAX_MS = 4000;
const INITIAL_SIZE = { cols: 120, rows: 36 };

/** Per-agent state the browser doesn't see. */
type Runtime = {
  /** The live process's token. Revoked as soon as the process stops being this agent's. */
  token: string | null;
  transcriptPath: string | null;
  startedAt: number;
  quickExits: number;
  retryTimer: ReturnType<typeof setTimeout> | null;
  /** The live process has reported its conversation at least once. */
  adopted: boolean;
  /** The highest event sequence number accepted from the live process. */
  lastSeq: number;
};

type Owner = { kind: "agent"; agentId: string } | { kind: "shell"; workspaceId: string };

type Ops = {
  ended(entry: TerminalEntry): void;
  closed(agentId: string, terminalId: string): void;
  retry(agentId: string): void;
  beforeShutdown(): void;
};

type State = {
  model: Model;
  runtime: Map<string, Runtime>;
  /** Token → the terminal it was issued to. */
  tokens: Map<string, string>;
  /** Terminal → what it belongs to. */
  owners: Map<string, Owner>;
  shuttingDown: boolean;
  /** The server's own origin, given to agents so their hooks can reach it. */
  url: string;
  onChange: (model: Model) => void;
  changeScheduled: boolean;
  ops: Ops;
};

const globals = globalThis as typeof globalThis & { __hypercubeSessions?: State };
const state: State = (globals.__hypercubeSessions ??= {
  model: emptyModel(),
  runtime: new Map(),
  tokens: new Map(),
  owners: new Map(),
  shuttingDown: false,
  url: "",
  onChange: () => {},
  changeScheduled: false,
  ops: undefined as unknown as Ops,
});

export class ModelError extends Error {}

export function getModel() {
  return state.model;
}

/** Where to push the model after changes, and the server's origin for agent hooks. */
export function bindModel(options: { url: string; onChange: (model: Model) => void }) {
  state.url = options.url;
  state.onChange = options.onChange;
}

/** Pushes the model once, after the current handler. */
function changed() {
  state.model.version += 1;
  if (state.changeScheduled) return;
  state.changeScheduled = true;
  queueMicrotask(flushModelChange);
}

/** Pushes a pending change now, so it reaches browsers before the reply to the request that made it. */
export function flushModelChange() {
  if (!state.changeScheduled) return;
  state.changeScheduled = false;
  state.onChange(state.model);
}

const newTerminalId = () => `t-${crypto.randomUUID()}`;
const newToken = () => Buffer.from(crypto.getRandomValues(new Uint8Array(32))).toString("hex");

function workspace(id: string) {
  const found = state.model.workspaces.find(workspace => workspace.id === id);
  if (!found) throw new ModelError("No such workspace");
  return found;
}

function session(id: string) {
  const found = state.model.sessions[id];
  if (!found) throw new ModelError("No such session");
  return found;
}

function runtime(agent: Agent) {
  let found = state.runtime.get(agent.id);
  if (!found) {
    found = { token: null, transcriptPath: null, startedAt: 0, quickExits: 0, retryTimer: null, adopted: false, lastSeq: 0 };
    state.runtime.set(agent.id, found);
  }
  return found;
}

function workspaceOf(agent: Agent) {
  return workspace(session(agent.sessionId).workspaceId);
}

// --- Spawning ---------------------------------------------------------------

function refuseDuringShutdown() {
  if (state.shuttingDown) throw new ModelError("The server is shutting down");
}

/** Starts the agent's process. Throws, changing nothing, if it can't be started. */
function spawnAgent(agent: Agent, cwd: string) {
  refuseDuringShutdown();
  const rt = runtime(agent);
  const { command, env } = harnessCommand(agent, rt.transcriptPath);
  const terminalId = newTerminalId();
  const token = newToken();
  // Registered before the spawn, so the process can call in as soon as it starts.
  state.tokens.set(token, terminalId);
  state.owners.set(terminalId, { kind: "agent", agentId: agent.id });
  try {
    createTerminal({
      id: terminalId,
      ...INITIAL_SIZE,
      cwd,
      command,
      env: { ...env, HYPERCUBE_URL: state.url, HYPERCUBE_TOKEN: token },
    });
  } catch (error) {
    state.tokens.delete(token);
    state.owners.delete(terminalId);
    throw error;
  }
  rt.token = token;
  rt.startedAt = Date.now();
  rt.adopted = false;
  rt.lastSeq = 0;
  agent.terminalId = terminalId;
  agent.state = "running";
  agent.failure = null;
}

/** Starts the agent, or marks it failed if it can't be started. */
function startAgent(agent: Agent) {
  if (state.shuttingDown) return;
  try {
    spawnAgent(agent, workspaceOf(agent).path);
  } catch (error) {
    agent.state = "failed";
    agent.failure = `Could not start: ${error instanceof Error ? error.message : String(error)}`;
  }
}

function spawnShell(ws: Workspace, path = ws.path) {
  refuseDuringShutdown();
  const terminalId = newTerminalId();
  state.owners.set(terminalId, { kind: "shell", workspaceId: ws.id });
  try {
    createTerminal({ id: terminalId, ...INITIAL_SIZE, cwd: path });
  } catch (error) {
    state.owners.delete(terminalId);
    throw error;
  }
  ws.shell = { terminalId, exitCode: null };
}

/** The agent's process is no longer its own: nothing it reports is accepted from now on. */
function revokeToken(agent: Agent) {
  const rt = runtime(agent);
  if (rt.token) state.tokens.delete(rt.token);
  rt.token = null;
}

function clearRetry(agent: Agent) {
  const rt = runtime(agent);
  if (rt.retryTimer) clearTimeout(rt.retryTimer);
  rt.retryTimer = null;
}

function stopAgent(agent: Agent) {
  switch (agent.state) {
    case "running": {
      const terminalId = agent.terminalId!;
      revokeToken(agent);
      agent.state = "stopping";
      void closeTerminal(terminalId).then(() => state.ops.closed(agent.id, terminalId));
      return;
    }
    case "restarting":
      clearRetry(agent);
      agent.state = "stopped";
      return;
    case "failed":
      agent.state = "stopped";
      agent.failure = null;
      return;
    // A stopping agent finishes stopping; stopped has nothing to do.
  }
}

// --- Operations (from browsers) -----------------------------------------------

function expandPath(input: string) {
  const trimmed = input.trim();
  if (trimmed === "~") return homedir();
  if (trimmed.startsWith("~/")) return homedir() + trimmed.slice(1);
  return trimmed;
}

export function addWorkspace(input: string) {
  const expanded = expandPath(input);
  if (!expanded || !isAbsolute(expanded)) throw new ModelError("Enter an absolute path or one starting with ~");
  let path: string;
  try {
    path = realpathSync(expanded);
  } catch {
    throw new ModelError(`${expanded} does not exist`);
  }
  if (!statSync(path).isDirectory()) throw new ModelError(`${path} is not a directory`);
  if (state.model.workspaces.some(ws => ws.path === path)) throw new ModelError(`${path} is already open`);

  const ws: Workspace = {
    id: crypto.randomUUID(),
    name: basename(path) || path,
    path,
    nextSessionNumber: 1,
    sessionIds: [],
    shell: { terminalId: null, exitCode: null },
  };
  // Only a workspace whose shell started is added.
  spawnShell(ws);
  state.model.workspaces.push(ws);
  changed();
  return ws.id;
}

export function createSession(workspaceId: string, harness: Harness) {
  if (harness !== "claude" && harness !== "opencode") throw new ModelError("Unknown harness");
  const ws = workspace(workspaceId);
  const sessionId = crypto.randomUUID();
  const agent: Agent = {
    id: crypto.randomUUID(),
    sessionId,
    role: "lead",
    harness,
    name: HARNESS_LABELS[harness],
    conversationId: newConversationId(harness),
    state: "stopped",
    terminalId: null,
    failure: null,
  };
  // Only a session whose lead started is added, and only then is its number used.
  try {
    spawnAgent(agent, ws.path);
  } catch (error) {
    state.runtime.delete(agent.id);
    throw new ModelError(`Could not start ${agent.name}: ${error instanceof Error ? error.message : String(error)}`);
  }
  const created: Session = {
    id: sessionId,
    workspaceId,
    name: `Untitled ${ws.nextSessionNumber++}`,
    state: "running",
    agentIds: [agent.id],
    createdAt: Date.now(),
  };
  state.model.sessions[sessionId] = created;
  state.model.agents[agent.id] = agent;
  ws.sessionIds.push(sessionId);
  changed();
  return sessionId;
}

export function startSession(sessionId: string) {
  refuseDuringShutdown();
  const target = session(sessionId);
  target.state = "running";
  for (const agentId of target.agentIds) {
    const agent = state.model.agents[agentId]!;
    if (agent.state === "stopped" || agent.state === "failed") {
      runtime(agent).quickExits = 0;
      startAgent(agent);
    }
    // A stopping agent is started when its close completes.
  }
  changed();
}

export function stopSession(sessionId: string) {
  const target = session(sessionId);
  target.state = "stopped";
  for (const agentId of target.agentIds) stopAgent(state.model.agents[agentId]!);
  changed();
}

export function retryAgent(agentId: string) {
  refuseDuringShutdown();
  const agent = state.model.agents[agentId];
  if (!agent) throw new ModelError("No such agent");
  if (agent.state !== "failed") return;
  session(agent.sessionId).state = "running";
  runtime(agent).quickExits = 0;
  startAgent(agent);
  changed();
}

export function restartShell(workspaceId: string) {
  const ws = workspace(workspaceId);
  if (ws.shell.terminalId) return;
  try {
    spawnShell(ws);
  } catch (error) {
    throw new ModelError(`Could not start the shell: ${error instanceof Error ? error.message : String(error)}`);
  }
  changed();
}

// --- Agent reports ------------------------------------------------------------

/** The agent whose live process holds this token, if any. */
export function agentForToken(token: string) {
  const terminalId = state.tokens.get(token);
  const owner = terminalId ? state.owners.get(terminalId) : undefined;
  if (owner?.kind !== "agent") return undefined;
  const agent = state.model.agents[owner.agentId];
  if (!agent || agent.terminalId !== terminalId || runtime(agent).token !== token) return undefined;
  return agent;
}

/** Claude Code's SessionStart hook. Returns false if the token isn't a live agent's. */
export function claudeSessionStarted(token: string, payload: { session_id?: unknown; transcript_path?: unknown }) {
  const agent = agentForToken(token);
  if (!agent || agent.harness !== "claude") return false;
  const rt = runtime(agent);
  if (typeof payload.transcript_path === "string") rt.transcriptPath = payload.transcript_path;
  const conversationId = typeof payload.session_id === "string" ? payload.session_id : null;
  if (conversationId && !rt.adopted) {
    // The first report from a process names the conversation it actually opened.
    rt.adopted = true;
    if (agent.conversationId !== conversationId) {
      agent.conversationId = conversationId;
      changed();
    }
  }
  return true;
}

/** The OpenCode plugin's route report. Returns false if the token isn't a live agent's. */
export function opencodeRouteChanged(token: string, payload: { seq?: unknown }) {
  const agent = agentForToken(token);
  if (!agent || agent.harness !== "opencode") return false;
  const rt = runtime(agent);
  if (typeof payload.seq !== "number" || payload.seq <= rt.lastSeq) return true;
  rt.lastSeq = payload.seq;
  return true;
}

// --- Events -------------------------------------------------------------------

state.ops = {
  ended(entry) {
    const owner = state.owners.get(entry.id);
    if (!owner) return;
    state.owners.delete(entry.id);

    if (owner.kind === "shell") {
      const ws = state.model.workspaces.find(ws => ws.id === owner.workspaceId);
      if (ws?.shell.terminalId === entry.id) ws.shell = { terminalId: null, exitCode: entry.exitCode };
      changed();
      return;
    }

    const agent = state.model.agents[owner.agentId];
    if (!agent || agent.terminalId !== entry.id) return;
    revokeToken(agent);
    agent.terminalId = null;
    const rt = runtime(agent);
    const parent = state.model.sessions[agent.sessionId];

    if (agent.role !== "lead" || parent?.state !== "running" || agent.state !== "running" || state.shuttingDown) {
      agent.state = "stopped";
    } else {
      rt.quickExits = Date.now() - rt.startedAt < QUICK_EXIT_MS ? rt.quickExits + 1 : 0;
      if (rt.quickExits >= MAX_QUICK_EXITS) {
        agent.state = "failed";
        agent.failure = `Exited ${rt.quickExits} times in a row (last exit code ${entry.exitCode ?? "none"})`;
      } else {
        agent.state = "restarting";
        const delay = Math.min(RETRY_BASE_MS * 2 ** Math.max(rt.quickExits - 1, 0), RETRY_MAX_MS);
        clearRetry(agent);
        rt.retryTimer = setTimeout(() => globals.__hypercubeSessions!.ops.retry(agent.id), delay);
      }
    }
    changed();
  },

  closed(agentId, terminalId) {
    state.owners.delete(terminalId);
    const agent = state.model.agents[agentId];
    if (!agent || agent.terminalId !== terminalId) return;
    agent.terminalId = null;
    // Started again while it was stopping.
    if (agent.state === "stopping" && state.model.sessions[agent.sessionId]?.state === "running") startAgent(agent);
    else if (agent.state === "stopping") agent.state = "stopped";
    changed();
  },

  retry(agentId) {
    const agent = state.model.agents[agentId];
    if (!agent) return;
    runtime(agent).retryTimer = null;
    if (agent.state !== "restarting") return;
    if (state.model.sessions[agent.sessionId]?.state === "running" && !state.shuttingDown) startAgent(agent);
    else agent.state = "stopped";
    changed();
  },

  beforeShutdown() {
    state.shuttingDown = true;
    for (const agent of Object.values(state.model.agents)) clearRetry(agent);
  },
};

setOwnerHooks({
  ended: entry => globals.__hypercubeSessions!.ops.ended(entry),
  beforeShutdown: () => globals.__hypercubeSessions!.ops.beforeShutdown(),
});

writeHarnessFiles();

/** Forgets the whole model. For tests; terminals must already be shut down. */
export function resetModelForTests() {
  for (const agent of Object.values(state.model.agents)) clearRetry(agent);
  state.model = emptyModel();
  state.runtime.clear();
  state.tokens.clear();
  state.owners.clear();
  state.shuttingDown = false;
}
