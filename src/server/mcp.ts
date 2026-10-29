import type { Server } from "bun";
import type { Agent } from "../model";
import { agentForToken, promptAgent } from "./agents";
import { loadConfig } from "./config";
import { ModelError } from "./errors";
import { getModel, removeWorker, startWorker } from "./sessions";
import { CONTAINER_HOST } from "./docker";
import { LOOPBACK_HOSTNAMES, isLoopbackRequest } from "./socket";

/**
 * Hypercube's MCP server, which every agent gets in `session/new`'s
 * `mcpServers`. Streamable HTTP, reduced to what a tools-only server needs:
 * each POST carries one JSON-RPC message and is answered with plain JSON.
 *
 * Who may call: the agent holding its own token, from this machine, with no
 * `Origin` header. Agents don't send one; browsers do, so a web page can't
 * reach this even if it learned a token. It is served by a listener of its own
 * on 127.0.0.1 (`src/index.ts`), apart from the app: Docker Desktop makes a
 * container's connections to `host.docker.internal` arrive from loopback, so
 * whatever that listener serves is open to every process in every container,
 * and it serves nothing else.
 */

const PROTOCOL_VERSIONS = ["2025-06-18", "2025-03-26", "2024-11-05"];

const LEAD_ONLY_TOOLS = new Set(["start_worker", "stop_worker"]);

const TOOLS = [
  {
    name: "list_agents",
    description: "List the agents you can message, with their ids, names, sessions and status.",
    inputSchema: { type: "object", properties: {} },
  },
  {
    name: "send_message",
    description:
      "Send a message to another agent. It is delivered as a prompt that says who sent it and how long ago. If the agent is working, its turn is interrupted after its current tool call and the message is delivered then. The agent answers with its own send_message.",
    inputSchema: {
      type: "object",
      properties: {
        to: { type: "string", description: "The agent's id or name. Use the id if several agents share a name." },
        text: { type: "string", description: "The message." },
      },
      required: ["to", "text"],
    },
  },
  {
    name: "list_profiles",
    description: "List the profiles a worker can be started on, with a description of when to use each.",
    inputSchema: { type: "object", properties: {} },
  },
  {
    name: "start_worker",
    description:
      "Lead only. Start a worker agent on a profile in a directory inside your clone. It appears in the session and works on its own; it can't see your conversation, so put its whole task in `message`. Returns the worker's id.",
    inputSchema: {
      type: "object",
      properties: {
        name: { type: "string", description: "The worker's name, unique in this session. Not \"Lead\"." },
        profile: { type: "string", description: "A profile name from list_profiles." },
        dir: { type: "string", description: "The directory it works in: your clone, or a git worktree inside it. A relative path is relative to your clone." },
        message: { type: "string", description: "The worker's first message: its task." },
      },
      required: ["name", "profile", "dir"],
    },
  },
  {
    name: "stop_worker",
    description: "Lead only. Stop one of your workers and remove it from the session.",
    inputSchema: {
      type: "object",
      properties: { id: { type: "string", description: "The worker's id or name." } },
      required: ["id"],
    },
  },
];

type RpcMessage = { id?: string | number | null; method?: unknown; params?: Record<string, unknown> };

/**
 * The agents `from` may message. A lead: its workers and the other leads in its
 * workspace. A worker: its lead, and nobody outside its session.
 */
function reachable(from: Agent) {
  const { sessions, agents } = getModel();
  const workspaceId = sessions[from.sessionId]?.workspaceId;
  return Object.values(agents).filter(agent => {
    if (agent.id === from.id) return false;
    if (from.role === "worker") return agent.sessionId === from.sessionId && agent.role === "lead";
    return agent.role === "lead" ? sessions[agent.sessionId]?.workspaceId === workspaceId : agent.sessionId === from.sessionId;
  });
}

/** "name (session)": how an agent is named to others, since leads all share the name "Lead". */
function label(agent: Agent) {
  return `${agent.name} (${getModel().sessions[agent.sessionId]?.name})`;
}

function listAgents(from: Agent) {
  return JSON.stringify(reachable(from).map(agent => ({ id: agent.id, name: agent.name, role: agent.role, session: getModel().sessions[agent.sessionId]?.name, status: agent.status, profile: agent.profile, error: agent.error })));
}

function sendMessage(from: Agent, args: Record<string, unknown> | undefined) {
  const { to, text } = args ?? {};
  if (typeof to !== "string" || typeof text !== "string") throw new ModelError("`to` and `text` must be strings");
  const peers = reachable(from);
  const wanted = to.trim().toLowerCase();
  const matches = peers.filter(agent => agent.id === to.trim() || agent.name.toLowerCase() === wanted);
  if (matches.length === 0) throw new ModelError(`No agent "${to}" that you can message. Call list_agents.`);
  if (matches.length > 1) throw new ModelError(`Several agents are named "${to}". Use one of their ids: ${matches.map(agent => agent.id).join(", ")}`);
  const recipient = matches[0]!;
  promptAgent(recipient.id, text, { id: from.id, label: label(from) });
  return `Sent to ${label(recipient)}.`;
}

async function listProfiles() {
  const { profiles } = await loadConfig();
  return JSON.stringify(profiles.map(({ name, description, model, effort }) => ({ name, description, model, effort })));
}

function requireLead(from: Agent) {
  if (from.role !== "lead") throw new ModelError("Only the lead can do that");
}

function stringArg(args: Record<string, unknown> | undefined, key: string) {
  const value = args?.[key];
  if (typeof value !== "string") throw new ModelError(`\`${key}\` must be a string`);
  return value;
}

async function startWorkerTool(from: Agent, args: Record<string, unknown> | undefined) {
  requireLead(from);
  const [name, profileName, dir] = [stringArg(args, "name"), stringArg(args, "profile"), stringArg(args, "dir")];
  const message = args?.message === undefined ? undefined : stringArg(args, "message");
  const { profiles } = await loadConfig();
  const profile = profiles.find(profile => profile.name === profileName);
  if (!profile) throw new ModelError(`No profile "${profileName}". The profiles are: ${profiles.map(profile => profile.name).join(", ")}`);
  const worker = await startWorker(from, name, profile, dir);
  if (worker.status === "exited") throw new ModelError(`Worker "${worker.name}" (id ${worker.id}) could not start: ${worker.error}. Stop it with stop_worker.`);
  if (message?.trim()) promptAgent(worker.id, message, { id: from.id, label: label(from) });
  return `Started worker "${worker.name}" (id ${worker.id}) on profile ${profile.name}. It is starting up${message?.trim() ? " and will begin on your message" : ""}.`;
}

async function stopWorkerTool(from: Agent, args: Record<string, unknown> | undefined) {
  requireLead(from);
  const wanted = stringArg(args, "id").trim();
  const { agents } = getModel();
  const workers = Object.values(agents).filter(agent => agent.role === "worker" && agent.sessionId === from.sessionId);
  const worker = workers.find(agent => agent.id === wanted) ?? workers.find(agent => agent.name.toLowerCase() === wanted.toLowerCase());
  if (!worker) throw new ModelError(`No worker "${wanted}" in your session. Call list_agents.`);
  await removeWorker(worker.id);
  return `Stopped worker "${worker.name}".`;
}

async function callTool(from: Agent, params: RpcMessage["params"]) {
  const args = params?.arguments as Record<string, unknown> | undefined;
  const tools: Record<string, () => string | Promise<string>> = {
    list_agents: () => listAgents(from),
    send_message: () => sendMessage(from, args),
    list_profiles: listProfiles,
    start_worker: () => startWorkerTool(from, args),
    stop_worker: () => stopWorkerTool(from, args),
  };
  const tool = typeof params?.name === "string" && Object.hasOwn(tools, params.name) ? tools[params.name] : undefined;
  if (!tool) return { code: -32602, message: `Unknown tool: ${String(params?.name)}` };
  try {
    return { result: { content: [{ type: "text", text: await tool() }] } };
  } catch (error) {
    // A failure the agent can act on goes back as the tool's result, not as a protocol error.
    if (!(error instanceof ModelError)) throw error;
    return { result: { content: [{ type: "text", text: error.message }], isError: true } };
  }
}

function rpc(id: RpcMessage["id"], body: { result: unknown } | { code: number; message: string }) {
  return Response.json("result" in body ? { jsonrpc: "2.0", id, result: body.result } : { jsonrpc: "2.0", id, error: { code: body.code, message: body.message } });
}

function bearerToken(req: Request) {
  return /^Bearer (\S+)$/i.exec(req.headers.get("authorization") ?? "")?.[1];
}

/** Agents in a Docker session reach this machine as `host.docker.internal`, so that name is accepted here too. */
const MCP_HOSTNAMES = new Set([...LOOPBACK_HOSTNAMES, CONTAINER_HOST]);

export async function handleMcp(req: Request, server: Server<undefined>) {
  if (!isLoopbackRequest(req, server.port!, server.requestIP(req)?.address, MCP_HOSTNAMES) || req.headers.has("origin")) {
    return new Response("Forbidden", { status: 403 });
  }
  const agent = agentForToken(bearerToken(req));
  if (!agent) return new Response("Unauthorized", { status: 401 });
  // There is no server-to-client stream or session to end.
  if (req.method !== "POST") return new Response("Method Not Allowed", { status: 405, headers: { allow: "POST" } });

  let message: RpcMessage;
  try {
    message = await req.json();
  } catch {
    return rpc(null, { code: -32700, message: "Parse error" });
  }
  if (typeof message !== "object" || message === null || Array.isArray(message)) return rpc(null, { code: -32600, message: "Send one JSON-RPC message per request" });
  // Notifications (like `initialized`) and responses get no answer.
  if (message.id === undefined || typeof message.method !== "string") return new Response(null, { status: 202 });

  switch (message.method) {
    case "initialize": {
      const asked = message.params?.protocolVersion;
      const protocolVersion = PROTOCOL_VERSIONS.find(version => version === asked) ?? PROTOCOL_VERSIONS[0];
      return rpc(message.id, { result: { protocolVersion, capabilities: { tools: {} }, serverInfo: { name: "hypercube", version: "0.0.0" } } });
    }
    case "ping":
      return rpc(message.id, { result: {} });
    case "tools/list":
      return rpc(message.id, { result: { tools: agent.role === "lead" ? TOOLS : TOOLS.filter(tool => !LEAD_ONLY_TOOLS.has(tool.name)) } });
    case "tools/call":
      return rpc(message.id, await callTool(agent, message.params));
    default:
      return rpc(message.id, { code: -32601, message: `Method not found: ${message.method}` });
  }
}
