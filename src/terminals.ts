import { FitAddon } from "@xterm/addon-fit";
import { Terminal } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";
import { SCROLLBACK } from "./protocol";
import { isSocketOpen, registerChannel, sendTerminal, unregisterChannel, type TerminalChannel } from "./socket";

/**
 * A view onto whichever session's terminal a panel currently shows. Views live
 * outside React, keyed by the panel, so one keeps its screen when its component
 * remounts. Views never create or close terminals; the model on the server does.
 */
export type TerminalView = {
  /** Element the terminal renders into; moved between containers. */
  element: HTMLDivElement;
  fit: () => void;
  focus: () => void;
  /** Shows this terminal (a session's id), or nothing. */
  setTerminal: (terminalId: string | null) => void;
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
    sendTerminal({ t: "attach", id: terminalId, cols: terminal.cols, rows: terminal.rows });
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
    if (attached && terminalId) sendTerminal({ t: "input", id: terminalId, data });
  });
  terminal.onResize(({ cols, rows }) => {
    if (attached && terminalId) sendTerminal({ t: "resize", id: terminalId, cols, rows });
  });

  const view: TerminalView = {
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
  };
  views.set(viewId, view);
  return view;
}

export function focusView(viewId: string) {
  views.get(viewId)?.focus();
}
