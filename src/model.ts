/**
 * The workspace/session/agent model, owned by the server and pushed whole to
 * every browser. Shared by both sides.
 */

export type Workspace = {
  id: string;
  name: string;
  /** Canonical (realpath) directory. */
  path: string;
  /** The next session's "Untitled N". Counts up, never reused. */
  nextSessionNumber: number;
  sessionIds: string[];
};

export type Session = {
  id: string;
  workspaceId: string;
  name: string;
  agentIds: string[];
  createdAt: number;
};

export type Agent = {
  id: string;
  sessionId: string;
  role: "lead" | "worker";
  name: string;
};

export type Model = {
  version: number;
  workspaces: Workspace[];
  sessions: Record<string, Session>;
  agents: Record<string, Agent>;
};

export const emptyModel = (): Model => ({ version: 0, workspaces: [], sessions: {}, agents: {} });
