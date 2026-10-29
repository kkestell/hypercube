/**
 * The workspace/session/agent model, owned by the server and pushed whole to
 * every browser. Shared by both sides.
 */

export type Harness = "claude" | "opencode";

export const HARNESS_LABELS: Record<Harness, string> = { claude: "Claude Code", opencode: "OpenCode" };

export type Workspace = {
  id: string;
  name: string;
  /** Canonical (realpath) directory. */
  path: string;
  /** The next session's "Untitled N". Counts up, never reused. */
  nextSessionNumber: number;
  sessionIds: string[];
  shell: Shell;
};

/** The workspace's plain shell. Not restarted automatically. */
export type Shell = { terminalId: string | null; exitCode: number | null };

export type Session = {
  id: string;
  workspaceId: string;
  name: string;
  state: "running" | "stopped";
  agentIds: string[];
  createdAt: number;
};

export type AgentState = "running" | "restarting" | "stopping" | "stopped" | "failed";

export type Agent = {
  id: string;
  sessionId: string;
  role: "lead" | "worker";
  harness: Harness;
  name: string;
  /** The harness's conversation: a UUID for Claude Code, `ses_…` for OpenCode. */
  conversationId: string;
  state: AgentState;
  /** The live process's terminal. Changes whenever the process is replaced. */
  terminalId: string | null;
  /** Why the agent is failed. */
  failure: string | null;
};

export type Model = {
  version: number;
  workspaces: Workspace[];
  sessions: Record<string, Session>;
  agents: Record<string, Agent>;
};

export const emptyModel = (): Model => ({ version: 0, workspaces: [], sessions: {}, agents: {} });
