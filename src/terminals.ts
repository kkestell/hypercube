import { FitAddon } from "@xterm/addon-fit";
import { Terminal } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";

/**
 * A terminal and its shell connection. Terminals live outside React so a
 * terminal keeps its scrollback and shell when its panel is dragged to
 * another group and the panel component remounts.
 */
export type TerminalHandle = {
  terminal: Terminal;
  /** Element the terminal renders into; moved between panel containers. */
  element: HTMLDivElement;
  fit: () => void;
  focus: () => void;
  dispose: () => void;
};

const terminals = new Map<string, TerminalHandle>();

export function getOrCreateTerminal(id: string, onExit: () => void): TerminalHandle {
  const existing = terminals.get(id);
  if (existing) return existing;

  const terminal = new Terminal({
    fontFamily: 'Menlo, Monaco, "Courier New", monospace',
    fontSize: 13,
    cursorBlink: true,
    theme: { background: "#1e1e1e" },
  });
  const fitAddon = new FitAddon();
  terminal.loadAddon(fitAddon);

  const element = document.createElement("div");
  element.className = "h-full w-full";
  terminal.open(element);

  const url = new URL("/pty", location.href);
  url.protocol = location.protocol === "https:" ? "wss:" : "ws:";
  url.searchParams.set("cols", String(terminal.cols));
  url.searchParams.set("rows", String(terminal.rows));
  const socket = new WebSocket(url);
  socket.binaryType = "arraybuffer";

  const encoder = new TextEncoder();
  let disposed = false;

  socket.onmessage = event => terminal.write(new Uint8Array(event.data));
  socket.onclose = () => {
    if (!disposed) onExit();
  };
  terminal.onData(data => {
    if (socket.readyState === WebSocket.OPEN) socket.send(encoder.encode(data));
  });
  terminal.onResize(({ cols, rows }) => {
    if (socket.readyState === WebSocket.OPEN) socket.send(JSON.stringify({ cols, rows }));
  });
  socket.onopen = () => socket.send(JSON.stringify({ cols: terminal.cols, rows: terminal.rows }));

  const handle: TerminalHandle = {
    terminal,
    element,
    fit: () => {
      // A detached or hidden element has no size to fit to.
      if (element.isConnected && element.clientWidth > 0 && element.clientHeight > 0) {
        fitAddon.fit();
      }
    },
    focus: () => terminal.focus(),
    dispose: () => {
      disposed = true;
      terminals.delete(id);
      socket.close();
      terminal.dispose();
    },
  };
  terminals.set(id, handle);
  return handle;
}

export function disposeTerminal(id: string) {
  terminals.get(id)?.dispose();
}

export function focusTerminal(id: string) {
  terminals.get(id)?.focus();
}
