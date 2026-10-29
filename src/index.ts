import { serve, type Server } from "bun";
import index from "./index.html";
import { handleMcp } from "./server/mcp";
import { restoreModel } from "./server/sessions";
import { bindServer, socket, upgradeSocket, type SocketData } from "./server/socket";

/*
 * Two listeners, because Docker Desktop lets a process in a container connect to
 * the host's 127.0.0.1 through `host.docker.internal`, and the connection arrives
 * from loopback like the browser's. Anything on that address is open to every
 * container, so only /mcp, which needs an agent's token, is there.
 *
 * The app (the page and /ws, which can start a shell) is on [::1]. Docker's proxy
 * connects over IPv4, so a container is refused. A browser opening
 * http://localhost:<port> tries ::1 as well as 127.0.0.1, so it still gets in.
 */
const server = serve<SocketData>({
  hostname: "::1",

  routes: {
    // Serve index.html for all unmatched routes.
    "/*": index,

    "/ws": (req: Request, server: Server<SocketData>) => upgradeSocket(req, server),
  },

  websocket: socket,

  development: process.env.NODE_ENV !== "production" && {
    // Enable browser hot reloading in development
    hmr: true,

    // Echo console logs from the browser to the server
    console: true,
  },
});

// Agents' MCP tools; see src/server/mcp.ts for who may call it. One port above the app's unless HYPERCUBE_MCP_PORT says otherwise.
const mcpServer = serve({
  hostname: "127.0.0.1",
  port: Number(process.env.HYPERCUBE_MCP_PORT) || server.port! + 1,
  routes: { "/mcp": (req: Request, mcp: Server<undefined>) => handleMcp(req, mcp) },
  fetch: () => new Response("Not Found", { status: 404 }),
});

const mcpUrl = new URL("/mcp", mcpServer.url).href;
bindServer(server, mcpUrl);
// Agents resumed here reach the MCP server through the URL bound above.
restoreModel();

console.log(`🚀 Server running at ${server.url}, agents' MCP server at ${mcpUrl}`);
