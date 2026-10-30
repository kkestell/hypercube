/**
 * The workspace/session/agent model, owned by the server and pushed whole to
 * every browser. Shared by both sides. A child names its parent (`workspaceId`,
 * `sessionId`) and nothing lists its children; they are in insertion order, so
 * a session's agents are its lead first, then its workers as they were started.
 */

export type Workspace = {
  id: string;
  /** GitHub repository, normalized to owner/name. */
  repo: string;
  /** The next session's "Untitled N". Counts up, never reused. */
  nextSessionNumber: number;
};

export type Session = {
  id: string;
  workspaceId: string;
  name: string;
  environment: "local" | "docker";
  clonePath: string;
  /** stopped: ready, but its agents' processes are stopped. Start resumes them. deleting: its agents and clone are being removed. */
  status: "cloning" | "ready" | "failed" | "stopped" | "deleting";
  error?: string;
  /** The session has a running shell. Set when the model is sent, from the terminal registry; never stored. */
  terminal?: true;
};

/** Work in a clone or worktree that exists nowhere else. `path` is relative to the clone, "." for the clone itself. */
export type UnsavedWork = {
  path: string;
  /** `git status --porcelain` lines. */
  uncommitted: string[];
  /** Commits on no remote branch, as "hash subject". */
  unpushed: string[];
  /** Set when the check itself failed. */
  error?: string;
};

/**
 * starting: the process is up or about to be, no ACP session yet.
 * idle: ready for a prompt. busy: a prompt is in flight.
 * waiting: a permission request is open. exited: the process is gone (its
 * session may be stopped, or it may have died; `error` says which).
 */
export type AgentStatus = "starting" | "idle" | "busy" | "waiting" | "exited";

/**
 * A prompt waiting its turn. `from` labels the agent that sent it, as "name (session)"; absent when the user did.
 * `fromId` is that agent's id, for replies. `sentAt` is when it was sent, in ms.
 */
/** `steer`: sent to steer, but queued because no turn was running yet (the agent was starting, or another prompt went first). */
export type QueuedPrompt = { text: string; from?: string; fromId?: string; sentAt: number; steer?: boolean };

/**
 * Steers waiting to interrupt the agent's turn, sent as one prompt before the
 * queue. `waitingFor` is the title of the tool call the agent is let finish
 * first; absent once the turn is being cancelled.
 */
export type PendingSteer = { prompts: QueuedPrompt[]; waitingFor?: string };

export type Agent = {
  id: string;
  sessionId: string;
  role: "lead" | "worker";
  name: string;
  /** The profile a worker was started with. Absent for leads, which use the config's `lead`. */
  profile?: string;
  status: AgentStatus;
  /** Why the agent exited, when it didn't exit on request. */
  error?: string;
  /** Where the agent runs: the clone for a lead, a directory inside it for a worker. */
  cwd: string;
  /** The agent's ACP session, once it has one. Kept so a new process can resume it. */
  acpSessionId?: string;
  /** Prompts waiting for the current turn to end, oldest first. */
  queue: QueuedPrompt[];
  /** A steer waiting for the turn to stop. */
  steer?: PendingSteer;
};

export type PermissionOption = { optionId: string; name: string; kind: string };

/** What a tool call produced. Diffs are unified, without the file header. */
export type ToolContent = { type: "text"; text: string } | { type: "diff"; path: string; patch: string };

export type ToolLocation = { path: string; line?: number };

/**
 * One entry in an agent's transcript. The server folds ACP updates into these;
 * an item with an id already in the transcript replaces it.
 */
export type TranscriptItem =
  /** A prompt we sent. `from` is the sending agent, "name (session)"; absent when the user wrote it. */
  | { id: string; kind: "user"; text: string; from?: string; steered?: boolean }
  | { id: string; kind: "agent"; text: string }
  | { id: string; kind: "thought"; text: string }
  | { id: string; kind: "tool"; title: string; toolKind: string; status: string; content: ToolContent[]; locations: ToolLocation[] }
  | {
      id: string;
      kind: "permission";
      requestId: string;
      title: string;
      options: PermissionOption[];
      /** The chosen option's id, or "cancelled". Absent while the request is open. */
      answer?: string;
    }
  | { id: string; kind: "notice"; text: string };

export type PlanEntry = { content: string; status: "pending" | "in_progress" | "completed" };

export type Usage = { used: number; size: number; cost?: { amount: number; currency: string } };

/** A select-type session config option. Option ids and values belong to the harness. */
export type ConfigOption = {
  id: string;
  name: string;
  /** "model", "thought_level", "mode", ... A hint for display only. */
  category?: string;
  currentValue: string;
  options: { value: string; name: string; description?: string }[];
};

export type Command = { name: string; description: string; hint?: string };

/**
 * What an agent's panel shows besides its transcript. A field is absent until
 * the agent has sent it. The server sends the whole of it each time, and the
 * browser replaces its copy; a new process starts with none.
 */
export type AgentInfo = {
  plan?: PlanEntry[];
  usage?: Usage;
  configOptions?: ConfigOption[];
  commands?: Command[];
};

export type Model = {
  workspaces: Workspace[];
  sessions: Record<string, Session>;
  agents: Record<string, Agent>;
};

export const emptyModel = (): Model => ({ workspaces: [], sessions: {}, agents: {} });
