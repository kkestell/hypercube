import { FitAddon } from "@xterm/addon-fit";
import { Terminal } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";
import { SCROLLBACK } from "./protocol";
import { isSocketOpen, registerChannel, send, unregisterChannel, type TerminalChannel } from "./terminalSocket";

/**
 * A view onto whichever server terminal an agent (or a workspace shell)
 * currently has. Views live outside React, keyed by what they show, so one
 * keeps its screen when its panel is dragged to another group and remounts,
 * and it switches terminals when the agent's process is replaced. Views never
 * create or close terminals; the model on the server does.
 */
export type TerminalView = {
  terminal: Terminal;
  /** Element the terminal renders into; moved between panel containers. */
  element: HTMLDivElement;
  fit: () => void;
  focus: () => void;
  /** Shows this terminal, or nothing. */
  setTerminal: (terminalId: string | null) => void;
  dispose: () => void;
};

const views = new Map<string, TerminalView>();

export function getOrCreateView(viewId: string): TerminalView {
  const existing = views.get(viewId);
  if (existing) return existing;

  const terminal = new Terminal({
    fontFamily: 'Menlo, Monaco, "Courier New", monospace',
    fontSize: 13,
    cursorBlink: true,
    scrollback: SCROLLBACK,
    theme: { background: "#1e1e1e" },
  });
  const fitAddon = new FitAddon();
  terminal.loadAddon(fitAddon);

  const element = document.createElement("div");
  element.className = "h-full w-full";
  terminal.open(element);

  let terminalId: string | null = null;
  /** The element has been fitted, so the server can be given the terminal's real size. */
  let sized = false;
  /** This connection has sent `attach` for the current terminal. */
  let attached = false;

  const attach = () => {
    if (!sized || !terminalId || attached || !isSocketOpen()) return;
    attached = true;
    send({ t: "attach", id: terminalId, cols: terminal.cols, rows: terminal.rows });
  };

  const channel: TerminalChannel = {
    onOpen: () => {
      attached = false;
      attach();
    },
    // The snapshot replaces whatever the screen shows, which matters after a reconnect.
    onSnapshot: snapshot => {
      terminal.write("\x1bc");
      terminal.write(snapshot);
    },
    onOutput: chunk => terminal.write(chunk),
  };

  terminal.onData(data => {
    if (attached && terminalId) send({ t: "input", id: terminalId, data });
  });
  terminal.onResize(({ cols, rows }) => {
    if (attached && terminalId) send({ t: "resize", id: terminalId, cols, rows });
  });

  const view: TerminalView = {
    terminal,
    element,
    fit: () => {
      // A detached or hidden element has no size to fit to.
      if (element.isConnected && element.clientWidth > 0 && element.clientHeight > 0) {
        fitAddon.fit();
        sized = true;
        attach();
      }
    },
    focus: () => terminal.focus(),
    setTerminal: next => {
      if (next === terminalId) return;
      if (terminalId) unregisterChannel(terminalId, channel);
      terminalId = next;
      attached = false;
      terminal.reset();
      if (next) registerChannel(next, channel);
    },
    dispose: () => {
      view.setTerminal(null);
      views.delete(viewId);
      terminal.dispose();
    },
  };
  views.set(viewId, view);
  return view;
}

export function focusView(viewId: string) {
  views.get(viewId)?.focus();
}
