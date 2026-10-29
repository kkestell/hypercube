import { afterAll, beforeAll, describe, expect, jest, mock, test } from "bun:test";
import { FRAME_OUTPUT, encodeFrame, type ClientMessage } from "./protocol";
import type { TerminalChannel } from "./terminalSocket";

class FakeWebSocket {
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSING = 2;
  static CLOSED = 3;
  static instances: FakeWebSocket[] = [];

  readyState = FakeWebSocket.CONNECTING;
  binaryType = "blob";
  sent: string[] = [];
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: unknown }) => void) | null = null;
  onclose: (() => void) | null = null;

  constructor(readonly url: URL) {
    FakeWebSocket.instances.push(this);
  }

  send(data: string) {
    this.sent.push(data);
  }

  close() {
    this.drop();
  }

  open() {
    this.readyState = FakeWebSocket.OPEN;
    this.onopen?.();
  }

  drop() {
    this.readyState = FakeWebSocket.CLOSED;
    this.onclose?.();
  }
}

const sockets = FakeWebSocket.instances;
const open = () => sockets.filter(ws => ws.readyState === FakeWebSocket.OPEN || ws.readyState === FakeWebSocket.CONNECTING);

function channel(): TerminalChannel & { opened: number; output: Uint8Array[] } {
  const state = {
    opened: 0,
    output: [] as Uint8Array[],
    onOpen() {
      state.opened += 1;
    },
    onSnapshot() {},
    onOutput(chunk: Uint8Array) {
      state.output.push(chunk);
    },
  };
  return state;
}

let socketModule: typeof import("./terminalSocket");

// Other test files in this process use the real WebSocket, so put the globals back afterwards.
const realGlobals = { WebSocket: globalThis.WebSocket, location: (globalThis as { location?: unknown }).location };

beforeAll(async () => {
  // Reconnect delays are driven by hand, so the tests don't wait on them.
  jest.useFakeTimers();
  Object.assign(globalThis, { WebSocket: FakeWebSocket, location: new URL("http://localhost:3000/") });
  socketModule = await import("./terminalSocket");
});

afterAll(() => {
  jest.useRealTimers();
  Object.assign(globalThis, realGlobals);
  if (realGlobals.location === undefined) delete (globalThis as { location?: unknown }).location;
});

// The module holds one socket for the tab, so these run in order against the same state.
describe("the tab's single socket", () => {
  const a = channel();
  const b = channel();
  const c = channel();

  test("registering terminals opens one socket, also while one is reconnecting", () => {
    socketModule.registerChannel("a", a);
    socketModule.registerChannel("b", b);
    expect(sockets).toHaveLength(1);
    expect(sockets[0]!.url.protocol).toBe("ws:");
    expect(sockets[0]!.url.pathname).toBe("/pty");

    sockets[0]!.open();
    expect(a.opened).toBe(1);
    expect(b.opened).toBe(1);

    // Registered during the reconnect delay: connects once, and the pending timer doesn't open a second socket.
    sockets[0]!.drop();
    expect(sockets).toHaveLength(1);
    socketModule.registerChannel("c", c);
    expect(sockets).toHaveLength(2);
    jest.advanceTimersByTime(5000);
    expect(sockets).toHaveLength(2);
    expect(open()).toEqual([sockets[1]!]);
  });

  test("messages go to the one open socket, and a stale socket's events are ignored", () => {
    sockets[1]!.open();
    expect(a.opened).toBe(2);
    expect(c.opened).toBe(1);

    socketModule.send({ t: "input", id: "a", data: "x" });
    expect(sockets[1]!.sent).toEqual([JSON.stringify({ t: "input", id: "a", data: "x" })]);
    expect(sockets[0]!.sent).toEqual([]);

    const stale = sockets[0]!;
    stale.onopen?.();
    expect(a.opened).toBe(2);
    stale.onmessage?.({ data: encodeFrame(FRAME_OUTPUT, "a", new TextEncoder().encode("stale")).buffer });
    expect(a.output).toHaveLength(0);
    // A stale close must not tear down or reschedule the live socket.
    stale.onclose?.();
    expect(open()).toEqual([sockets[1]!]);

    sockets[1]!.onmessage?.({ data: encodeFrame(FRAME_OUTPUT, "a", new TextEncoder().encode("live")).buffer });
    expect(a.output.map(chunk => new TextDecoder().decode(chunk))).toEqual(["live"]);
  });

  test("a socket that drops, even while connecting, is replaced by exactly one", () => {
    sockets[1]!.drop();
    jest.advanceTimersByTime(250);
    expect(sockets).toHaveLength(3);
    // Dropped before it opened: the next attempt comes after a longer delay and is still one socket.
    sockets[2]!.drop();
    jest.advanceTimersByTime(499);
    expect(sockets).toHaveLength(3);
    jest.advanceTimersByTime(1);
    expect(sockets).toHaveLength(4);
    expect(open()).toEqual([sockets[3]!]);

    sockets[3]!.open();
    socketModule.registerChannel("d", channel());
    expect(sockets).toHaveLength(4);
  });

  test("a request resolves with its result, and fails if the socket drops first", async () => {
    const live = sockets[3]!;
    const created = socketModule.request({ t: "addWorkspace", path: "/tmp" });
    const sent = JSON.parse(live.sent.at(-1)!);
    expect(sent).toMatchObject({ t: "addWorkspace", path: "/tmp" });
    live.onmessage?.({ data: JSON.stringify({ t: "result", req: sent.req, ok: true, id: "w1" }) });
    expect(await created).toBe("w1");

    const lost = socketModule.request({ t: "stopSession", sessionId: "s" });
    live.drop();
    await expect(lost).rejects.toThrow("Lost the connection");
  });
});

describe("a terminal view", () => {
  let terminals: typeof import("./terminals");
  const sentTo = (ws: FakeWebSocket) => ws.sent.map(data => JSON.parse(data) as ClientMessage);

  beforeAll(async () => {
    class FakeTerminal {
      cols = 80;
      rows = 24;
      loadAddon() {}
      open() {}
      write() {}
      focus() {}
      dispose() {}
      onData() {}
      onResize() {}
      reset() {}
    }
    mock.module("@xterm/xterm", () => ({ Terminal: FakeTerminal }));
    mock.module("@xterm/addon-fit", () => ({ FitAddon: class { fit() {} } }));
    mock.module("@xterm/xterm/css/xterm.css", () => ({}));
    const element = { className: "", isConnected: true, clientWidth: 100, clientHeight: 100 };
    Object.assign(globalThis, { document: { createElement: () => element } });
    terminals = await import("./terminals");
  });

  afterAll(() => {
    delete (globalThis as { document?: unknown }).document;
  });

  const lastOf = (ws: FakeWebSocket, t: string) => sentTo(ws).filter(m => m.t === t);

  test("a view attaches to the terminal it is given, never creates or closes one, and follows a replacement", () => {
    jest.advanceTimersByTime(5000);
    const live = open()[0]!;
    live.open();
    const view = terminals.getOrCreateView("agent-1");
    view.fit();
    expect(lastOf(live, "attach")).toEqual([]);

    view.setTerminal("t-1");
    expect(lastOf(live, "attach")).toEqual([{ t: "attach", id: "t-1", cols: 80, rows: 24 }]);

    // The agent's process was replaced.
    view.setTerminal("t-2");
    expect(lastOf(live, "detach")).toEqual([{ t: "detach", id: "t-1" }]);
    expect(lastOf(live, "attach").at(-1)).toEqual({ t: "attach", id: "t-2", cols: 80, rows: 24 });

    view.dispose();
    expect(lastOf(live, "detach").at(-1)).toEqual({ t: "detach", id: "t-2" });
    expect(sentTo(live).some(m => (m.t as string) === "close" || "create" in m)).toBe(false);
  });

  test("a terminal moving from one view to another stays attached", () => {
    const live = open()[0]!;
    const from = terminals.getOrCreateView("agent-a");
    const to = terminals.getOrCreateView("agent-b");
    from.fit();
    to.fit();
    from.setTerminal("t-moving");
    to.setTerminal("t-moving");
    from.setTerminal(null);
    expect(lastOf(live, "detach").filter(m => "id" in m && m.id === "t-moving")).toEqual([]);
    to.setTerminal(null);
    expect(lastOf(live, "detach").filter(m => "id" in m && m.id === "t-moving")).toHaveLength(1);
  });
});
