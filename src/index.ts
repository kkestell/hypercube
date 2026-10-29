import { serve, type Server } from "bun";
import index from "./index.html";
import { bindServer, socket, upgradeSocket, type SocketData } from "./server/socket";

const server = serve<SocketData>({
  // Only accept connections from this machine.
  hostname: "localhost",

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

bindServer(server);

console.log(`🚀 Server running at ${server.url}`);
