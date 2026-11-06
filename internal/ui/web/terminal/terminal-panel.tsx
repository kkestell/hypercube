// The panel's shell for the attached session. The server starts it when the
// panel first shows and keeps it running until it exits or the session is
// archived or deleted, so hiding the panel leaves it running. The server replays the shell's recent
// output on every connection, so a reconnect starts the terminal over. The
// panel closes when its shell exits or cannot connect.
import {useEffect, useRef} from 'react';
import {isDark} from '../app/appearance.ts';
import {closePanel} from '../app/workbench.ts';
import {useStore} from '../store.ts';
import {createTerminal, type TerminalStyle} from './xterm.ts';

interface Shell {
  focus(): void;
  restyle(style: TerminalStyle): void;
  close(): void;
}

// openShell connects a terminal in box to the session's shell, calling ended
// when the shell exits or cannot connect.
function openShell(box: HTMLElement, session: string, style: TerminalStyle, ended: () => void): Shell {
  const {term, fit, restyle, dispose} = createTerminal(box, style);
  const encoder = new TextEncoder();
  let socket: WebSocket | null = null, connected = false, exited = false, closed = false;
  const send = (data: Uint8Array<ArrayBuffer> | string) => {
    if (socket?.readyState === WebSocket.OPEN) socket.send(data);
  };
  term.onData(data => send(encoder.encode(data)));
  term.onBinary(data => send(Uint8Array.from(data, c => c.charCodeAt(0))));
  term.onResize(({cols, rows}) => send(JSON.stringify({cols, rows})));

  // A hidden panel has no size to fit.
  const resized = () => {
    if (box.clientWidth && box.clientHeight) fit.fit();
  };
  const observer = new ResizeObserver(resized);
  observer.observe(box);
  resized();

  const connect = () => {
    const params = new URLSearchParams({session, cols: String(term.cols), rows: String(term.rows)});
    socket = new WebSocket(new URL('terminal?' + params, location.href.replace(/^http/, 'ws')));
    socket.binaryType = 'arraybuffer';
    socket.onopen = () => {
      connected = true;
      term.reset();
    };
    socket.onmessage = e => {
      if (typeof e.data === 'string') exited = true;
      else if (e.data instanceof ArrayBuffer) term.write(new Uint8Array(e.data));
    };
    socket.onclose = () => {
      if (closed) return;
      if (exited || !connected) ended();
      else setTimeout(() => {
        if (!closed) connect();
      }, 1000);
    };
  };
  connect();

  return {
    focus: () => term.focus(),
    restyle(style) {
      restyle(style);
      resized();
    },
    close() {
      closed = true;
      socket?.close();
      observer.disconnect();
      dispose();
    },
  };
}

export function TerminalPanel() {
  const [state, store] = useStore();
  const box = useRef<HTMLDivElement>(null);
  const shell = useRef<Shell | null>(null);
  const session = state.session?.id;
  const shown = state.workbench.panel;
  const dark = isDark(state);
  const fontSize = state.config?.terminal_font_size ?? 13;

  useEffect(() => {
    if (!shown || !session || !box.current) return;
    shell.current ??= openShell(box.current, session, {dark, fontSize}, () => {
      shell.current?.close();
      shell.current = null;
      store.update(closePanel);
    });
    shell.current.focus();
  }, [shown]);

  useEffect(() => shell.current?.restyle({dark, fontSize}), [dark, fontSize]);

  useEffect(() => () => shell.current?.close(), []);

  return <div ref={box} className="terminal-panel h-full min-h-0 overflow-hidden p-2" />;
}
