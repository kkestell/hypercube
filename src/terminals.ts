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
  /** Fits to the element's size; attaches if that was holding it back. */
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
  /** This connection has sent `attach` for the current terminal. */
  let attached = false;

  /** Fits the terminal to its element, if the element has a size to fit to. A detached or hidden one has none. */
  const fitNow = () => {
    if (!element.isConnected || element.clientWidth === 0 || element.clientHeight === 0) return false;
    fitAddon.fit();
    return true;
  };

  // The server is told the size the terminal has at that moment, so attach fits first.
  const attach = () => {
    if (!terminalId || attached || !isSocketOpen() || !fitNow()) return;
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
    fit: () => (attached ? fitNow() : attach()),
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
