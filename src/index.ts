import { serve, type Subprocess } from "bun";
import index from "./index.html";

type PtySocketData = {
  cols: number;
  rows: number;
  proc?: Subprocess;
};

const shell = process.env.SHELL ?? "/bin/zsh";

const server = serve({
  // The PTY endpoint runs a shell, so only accept connections from this machine.
  hostname: "localhost",

  routes: {
    // Serve index.html for all unmatched routes.
    "/*": index,

    "/pty": (req, server) => {
      // Reject cross-site pages that try to open a shell through the user's browser.
      const origin = req.headers.get("origin");
      if (!origin || new URL(origin).host !== req.headers.get("host")) {
        return new Response("Forbidden", { status: 403 });
      }

      const url = new URL(req.url);
      const data: PtySocketData = {
        cols: Number(url.searchParams.get("cols")) || 80,
        rows: Number(url.searchParams.get("rows")) || 24,
      };
      if (server.upgrade(req, { data })) return;
      return new Response("Expected a WebSocket upgrade", { status: 400 });
    },
  },

  websocket: {
    data: {} as PtySocketData,

    open(ws) {
      const proc = Bun.spawn([shell, "-l"], {
        cwd: process.env.HOME,
        env: { ...process.env, TERM: "xterm-256color", COLORTERM: "truecolor" },
        terminal: {
          cols: ws.data.cols,
          rows: ws.data.rows,
          data: (_terminal, output) => ws.send(output),
        },
      });
      ws.data.proc = proc;
      proc.exited.then(() => ws.close());
    },

    // Binary frames are keyboard input; text frames are JSON resize messages.
    message(ws, message) {
      const terminal = ws.data.proc?.terminal;
      if (!terminal) return;
      if (typeof message === "string") {
        const { cols, rows } = JSON.parse(message);
        terminal.resize(cols, rows);
      } else {
        terminal.write(message);
      }
    },

    close(ws) {
      ws.data.proc?.kill();
      ws.data.proc?.terminal?.close();
    },
  },

  development: process.env.NODE_ENV !== "production" && {
    // Enable browser hot reloading in development
    hmr: true,

    // Echo console logs from the browser to the server
    console: true,
  },
});

console.log(`🚀 Server running at ${server.url}`);
