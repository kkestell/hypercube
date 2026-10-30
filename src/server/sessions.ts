import { emptyModel, type Agent, type Model, type Session, type Workspace } from "../model";
import { forgetAgents, restoreTranscripts, startAgent, stopAgents } from "./agents";
import type { Profile } from "./config";
import { clonePathFor, environmentFor, localEnvironment, resolveDir } from "./environment";
import { ModelError, UnsavedWorkError } from "./errors";
import { loadStore, openStore, saveModel } from "./store";
import { closeTerminal, createTerminal, getTerminal } from "./terminals";
import { findUnsavedWork } from "./unsaved";

/**
 * The workspace/session/agent model. The server owns it, in memory; browsers
 * get it pushed and only ask for changes. Changes are written to the database
 * (`store.ts`) as they are pushed, and loaded again at startup. State lives on
 * globalThis, so a `bun --hot` reload keeps the model.
 */

type State = {
  model: Model;
  onChange: (model: Model) => void;
  changeScheduled: boolean;
  /** Whether this process has loaded the database. */
  restored: boolean;
};

const globals = globalThis as typeof globalThis & { __hypercubeModel?: State };
const state: State = (globals.__hypercubeModel ??= { model: emptyModel(), onChange: () => {}, changeScheduled: false, restored: false });

export function getModel() {
  return state.model;
}

/** The model as browsers get it: each session's `terminal` comes from the terminal registry. */
export function getPushedModel(): Model {
  const sessions = Object.entries(state.model.sessions).map(([id, session]) => [id, getTerminal(id) ? { ...session, terminal: true as const } : session]);
  return { ...state.model, sessions: Object.fromEntries(sessions) };
}

/** The agents of a session, its lead first and then its workers in the order they were started. */
export function agentsOf(session: Session) {
  return Object.values(state.model.agents).filter(agent => agent.sessionId === session.id);
}

/** Where to push the model after changes. */
export function bindModel(options: { onChange: (model: Model) => void }) {
  state.onChange = options.onChange;
}

/** Pushes the model once, after the current handler. */
export function changed() {
  if (state.changeScheduled) return;
  state.changeScheduled = true;
  queueMicrotask(flushModelChange);
}

/** Stores and pushes a pending change now, so it reaches browsers before the reply to the request that made it. */
export function flushModelChange() {
  if (!state.changeScheduled) return;
  state.changeScheduled = false;
  try {
    saveModel(state.model);
  } catch (error) {
    // The server keeps working; the change is stored by the next flush that can.
    console.error("Could not save the model:", error);
  }
  state.onChange(getPushedModel());
}

/**
 * Loads the database and resumes the agents of every session that was running.
 * Once per server process: a `bun --hot` reload keeps the live model, and one
 * made by code from before the database has nothing to load into.
 */
export function restoreModel() {
  if (state.restored !== false) return;
  state.restored = true;
  openStore();
  const saved = loadStore();
  const model = emptyModel();
  model.workspaces = saved.workspaces;
  for (const session of saved.sessions) model.sessions[session.id] = session;
  for (const stored of saved.agents) model.agents[stored.id] = { ...stored, status: "exited", queue: [] };
  state.model = model;
  restoreTranscripts(saved.items);

  for (const session of saved.sessions) {
    if (session.status === "cloning" || session.status === "deleting") {
      session.error = `Hypercube stopped while this session was ${session.status === "cloning" ? "cloning" : "being deleted"}`;
      session.status = "failed";
    }
    // A ready session's agents are resumed once its environment is up; `startAgent` waits for that.
    for (const agent of agentsOf(session)) {
      if (session.status === "failed") agent.error = session.error;
      else if (session.status === "ready") start(agent);
    }
  }
  changed();
}

function workspace(id: string) {
  const found = state.model.workspaces.find(workspace => workspace.id === id);
  if (!found) throw new ModelError("No such workspace");
  return found;
}

function normalizeRepo(input: string) {
  const match = /^(?:https:\/\/github\.com\/|git@github\.com:)?([A-Za-z0-9][A-Za-z0-9-]*)\/([A-Za-z0-9_.-]+?)(?:\.git)?\/?$/.exec(input.trim());
  if (!match || match[2] === "." || match[2] === "..") {
    throw new ModelError("Enter a GitHub repository as owner/name or a GitHub URL");
  }
  return `${match[1]}/${match[2]}`;
}

export async function addWorkspace(input: string) {
  const repo = normalizeRepo(input);
  const checkDuplicate = () => {
    if (state.model.workspaces.some(ws => ws.repo === repo)) throw new ModelError(`${repo} is already open`);
  };
  checkDuplicate();
  try {
    await localEnvironment.run(["gh", "repo", "view", repo, "--json", "nameWithOwner"]);
  } catch (error) {
    throw new ModelError(`Cannot open ${repo}: ${error instanceof Error ? error.message : String(error)}`);
  }
  // Another request may have added the repo while gh was running.
  checkDuplicate();

  const ws: Workspace = { id: crypto.randomUUID(), repo, nextSessionNumber: 1 };
  state.model.workspaces.push(ws);
  changed();
  return ws.id;
}

/** Creates a session named "Untitled N" with its lead agent, in the given environment. */
export function createSession(workspaceId: string, environment: Session["environment"]) {
  if (environment !== "local" && environment !== "docker") throw new ModelError("The environment must be local or docker");
  const ws = workspace(workspaceId);
  const sessionId = crypto.randomUUID();
  const clonePath = clonePathFor(environment, sessionId);
  const lead: Agent = { id: crypto.randomUUID(), sessionId, role: "lead", name: "Lead", cwd: clonePath, status: "starting", queue: [] };
  const created: Session = {
    id: sessionId,
    workspaceId,
    name: `Untitled ${ws.nextSessionNumber++}`,
    environment,
    clonePath,
    status: "cloning",
  };
  state.model.sessions[sessionId] = created;
  state.model.agents[lead.id] = lead;
  changed();
  void setupSession(created);
  return sessionId;
}

/** Starts the agent's process in the background: a new session for a new agent, a resumed one for an agent that has one. */
function start(agent: Agent, profile?: Profile) {
  agent.status = "starting";
  agent.error = undefined;
  const session = state.model.sessions[agent.sessionId]!;
  const lead = agentsOf(session).find(candidate => candidate.role === "lead")!;
  void startAgent(agent, session, workspace(session.workspaceId).repo, changed, { lead, profile });
}

async function setupSession(session: Session) {
  try {
    await environmentFor(session).setup(workspace(session.workspaceId).repo, session.clonePath);
    session.status = "ready";
  } catch (error) {
    session.status = "failed";
    session.error = error instanceof Error ? error.message : String(error);
  }
  if (state.model.sessions[session.id] !== session) return;
  for (const agent of agentsOf(session)) {
    if (session.status === "ready") {
      start(agent);
    } else {
      agent.status = "exited";
      agent.error = session.error;
    }
  }
  changed();
}

/**
 * Adds a worker to the lead's session and starts it on `profile` in `dir`, a
 * directory inside the clone. Worker names are unique in the session, so
 * messages can name them, and "Lead" is the lead's.
 */
export async function startWorker(lead: Agent, name: string, profile: Profile, dir: string) {
  const session = state.model.sessions[lead.sessionId];
  if (!session || lead.role !== "lead") throw new ModelError("Only a lead can start workers");
  name = name.trim();
  if (!name) throw new ModelError("A worker needs a name");
  const taken = () => agentsOf(session).some(agent => agent.name.toLowerCase() === name.toLowerCase());
  if (taken()) throw new ModelError(`This session already has an agent named "${name}"`);
  let cwd: string;
  try {
    cwd = await resolveDir(environmentFor(session), session.clonePath, dir);
  } catch (error) {
    throw new ModelError(error instanceof Error ? error.message : String(error));
  }
  // The session may have been deleted, or another worker named, while we looked at the directory.
  if (state.model.sessions[session.id] !== session || session.status !== "ready") throw new ModelError("This session is going away");
  if (taken()) throw new ModelError(`This session already has an agent named "${name}"`);

  const worker: Agent = { id: crypto.randomUUID(), sessionId: session.id, role: "worker", name, profile: profile.name, cwd, status: "starting", queue: [] };
  state.model.agents[worker.id] = worker;
  changed();
  start(worker, profile);
  return worker;
}

/** Stops a worker and removes it from its session, with its transcript. */
export async function removeWorker(agentId: string) {
  const worker = state.model.agents[agentId];
  if (worker?.role !== "worker") throw new ModelError("No such worker");
  delete state.model.agents[agentId];
  changed();
  await stopAgents([agentId]);
  forgetAgents([agentId]);
}

/** Starts the session's shell in its clone. */
export function startTerminal(sessionId: string) {
  const session = state.model.sessions[sessionId];
  if (!session) throw new ModelError("No such session");
  if (session.status !== "ready") throw new ModelError("The session isn't running");
  if (getTerminal(sessionId)) throw new ModelError("This session already has a terminal");
  const environment = environmentFor(session);
  try {
    createTerminal(session.id, 80, 24, onData => environment.spawnPty(environment.loginShell(), session.clonePath, { cols: 80, rows: 24, onData }));
  } catch (error) {
    throw new ModelError(`Could not start the terminal: ${error instanceof Error ? error.message : String(error)}`);
  }
  changed();
}

/** Marks the session's agents exited and stops them, workers included, and its shell. Their records and transcripts stay. */
async function halt(session: Session) {
  const agents = agentsOf(session);
  for (const agent of agents) {
    agent.status = "exited";
    agent.error = undefined;
    agent.queue = [];
    agent.steer = undefined;
  }
  changed();
  await Promise.all([closeTerminal(session.id), stopAgents(agents.map(agent => agent.id))]);
}

/** Stops the session's agents, workers included. Their records and transcripts stay, so Start can resume them. */
export async function stopSession(sessionId: string) {
  const session = state.model.sessions[sessionId];
  if (!session) throw new ModelError("No such session");
  if (session.status !== "ready") throw new ModelError("Only a running session can be stopped");
  session.status = "stopped";
  await halt(session);
}

/** Starts a stopped session's agents again, each resuming its ACP session. */
export function startSession(sessionId: string) {
  const session = state.model.sessions[sessionId];
  if (!session) throw new ModelError("No such session");
  if (session.status !== "stopped") throw new ModelError("This session is not stopped");
  session.status = "ready";
  for (const agent of agentsOf(session)) start(agent);
  changed();
}

/** Starts an exited agent's process again, resuming its ACP session. */
export function restartAgent(agentId: string) {
  const agent = state.model.agents[agentId];
  if (!agent) throw new ModelError("No such agent");
  if (state.model.sessions[agent.sessionId]!.status !== "ready") throw new ModelError("Start the session first");
  if (agent.status !== "exited") throw new ModelError("This agent is running");
  start(agent);
  changed();
}

/**
 * Deletes the session and its clone. Refused, with what would be lost, while
 * the clone or a worktree in it has uncommitted changes or commits on no
 * remote branch, unless `confirmed`.
 */
export async function deleteSession(sessionId: string, confirmed = false) {
  const session = state.model.sessions[sessionId];
  if (!session) throw new ModelError("No such session");
  if (session.status === "cloning") throw new ModelError("Wait for cloning to finish before deleting this session");
  if (session.status === "deleting") throw new ModelError("This session is already being deleted");
  if (!confirmed && session.status !== "failed") {
    const unsaved = await findUnsavedWork(environmentFor(session), session.clonePath);
    if (unsaved.length > 0) throw new UnsavedWorkError(unsaved);
  }
  const before = session.status;
  // Nothing can start in it from here on: the guards on `ready` and `stopped` refuse.
  session.status = "deleting";
  const agents = agentsOf(session);
  try {
    await halt(session);
    await environmentFor(session).teardown(session.clonePath);
  } catch (error) {
    // Its agents are stopped, and so is it.
    session.status = before === "failed" ? before : "stopped";
    changed();
    throw error;
  }
  forgetAgents(agents.map(agent => agent.id));
  for (const agent of agents) delete state.model.agents[agent.id];
  delete state.model.sessions[sessionId];
  changed();
}

/** Forgets the whole model. For tests. */
export function resetModelForTests() {
  state.model = emptyModel();
  state.changeScheduled = false;
}
