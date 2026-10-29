import { serve, type Server } from "bun";
import index from "./index.html";
import { AGENT_API_PATHS, handleAgentRequest } from "./server/agentApi";
import { bindTerminalServer, terminalSocket, upgradeTerminalSocket, type SocketData } from "./server/socket";

const server = serve<SocketData>({
  // The terminal socket runs shells, so only accept connections from this machine.
  hostname: "localhost",

  routes: {
    // Serve index.html for all unmatched routes.
    "/*": index,

    "/pty": (req: Request, server: Server<SocketData>) => upgradeTerminalSocket(req, server),

    // Agents' hooks and plugins report here; see agentApi.ts for how they are authenticated.
    ...Object.fromEntries(AGENT_API_PATHS.map(path => [path, (req: Request, server: Server<SocketData>) => handleAgentRequest(req, server)])),
  },

  websocket: terminalSocket,

  development: process.env.NODE_ENV !== "production" && {
    // Enable browser hot reloading in development
    hmr: true,

    // Echo console logs from the browser to the server
    console: true,
  },
});

bindTerminalServer(server);

console.log(`🚀 Server running at ${server.url}`);
