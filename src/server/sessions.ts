import { realpathSync, statSync } from "node:fs";
import { homedir } from "node:os";
import { basename, isAbsolute } from "node:path";
import { emptyModel, type Agent, type Model, type Session, type Workspace } from "../model";

/**
 * The workspace/session/agent model. The server owns it, in memory; browsers
 * get it pushed and only ask for changes. State lives on globalThis, so a
 * `bun --hot` reload keeps the model.
 */

type State = {
  model: Model;
  onChange: (model: Model) => void;
  changeScheduled: boolean;
};

const globals = globalThis as typeof globalThis & { __hypercubeModel?: State };
const state: State = (globals.__hypercubeModel ??= { model: emptyModel(), onChange: () => {}, changeScheduled: false });

export class ModelError extends Error {}

export function getModel() {
  return state.model;
}

/** Where to push the model after changes. */
export function bindModel(options: { onChange: (model: Model) => void }) {
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

function workspace(id: string) {
  const found = state.model.workspaces.find(workspace => workspace.id === id);
  if (!found) throw new ModelError("No such workspace");
  return found;
}

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

  const ws: Workspace = { id: crypto.randomUUID(), name: basename(path) || path, path, nextSessionNumber: 1, sessionIds: [] };
  state.model.workspaces.push(ws);
  changed();
  return ws.id;
}

/** Creates a session named "Untitled N" with its lead agent. */
export function createSession(workspaceId: string) {
  const ws = workspace(workspaceId);
  const sessionId = crypto.randomUUID();
  const lead: Agent = { id: crypto.randomUUID(), sessionId, role: "lead", name: "Lead" };
  const created: Session = {
    id: sessionId,
    workspaceId,
    name: `Untitled ${ws.nextSessionNumber++}`,
    agentIds: [lead.id],
    createdAt: Date.now(),
  };
  state.model.sessions[sessionId] = created;
  state.model.agents[lead.id] = lead;
  ws.sessionIds.push(sessionId);
  changed();
  return sessionId;
}

/** Forgets the whole model. For tests. */
export function resetModelForTests() {
  state.model = emptyModel();
  state.changeScheduled = false;
}
