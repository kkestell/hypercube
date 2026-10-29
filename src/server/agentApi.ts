import type { Server } from "bun";
import { claudeSessionStarted, opencodeRouteChanged } from "./sessions";
import { isLoopbackRequest } from "./socket";

/**
 * HTTP endpoints that agents' hooks and plugins call to report which
 * conversation they are on. Browsers never call these: a request must come
 * from this machine to a loopback host on our port, carry no Origin (every
 * browser sends one on a cross-origin POST, and we answer no preflight, so a
 * page can't set the Authorization header anyway), and present the token of a
 * process that is still an agent's.
 */

const MAX_BODY_BYTES = 64 * 1024;

const handlers: Record<string, (token: string, body: Record<string, unknown>) => boolean> = {
  "/api/agent/claude-session-start": claudeSessionStarted,
  "/api/agent/opencode-route": opencodeRouteChanged,
};

export const AGENT_API_PATHS = Object.keys(handlers);

const forbidden = () => new Response(null, { status: 403 });

function bearer(req: Request) {
  const match = /^Bearer ([0-9a-f]{64})$/.exec(req.headers.get("authorization") ?? "");
  return match?.[1];
}

export async function handleAgentRequest(req: Request, server: Server<unknown>) {
  const handler = handlers[new URL(req.url).pathname];
  if (!handler || req.method !== "POST") return new Response(null, { status: 404 });
  if (!isLoopbackRequest(req, server.port!, server.requestIP(req)?.address) || req.headers.has("origin")) return forbidden();
  const token = bearer(req);
  if (!token) return forbidden();

  if (Number(req.headers.get("content-length") ?? 0) > MAX_BODY_BYTES) return new Response(null, { status: 413 });
  const bytes = new Uint8Array(await req.arrayBuffer());
  if (bytes.length > MAX_BODY_BYTES) return new Response(null, { status: 413 });
  let body: unknown;
  try {
    body = JSON.parse(new TextDecoder().decode(bytes));
  } catch {
    return new Response(null, { status: 400 });
  }
  if (typeof body !== "object" || body === null) return new Response(null, { status: 400 });

  // The handler looks the token up in the live model and rejects one that no longer belongs to an agent.
  return handler(token, body as Record<string, unknown>) ? new Response(null, { status: 204 }) : forbidden();
}
