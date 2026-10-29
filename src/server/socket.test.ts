import { afterAll, afterEach, beforeAll, describe, expect, test } from "bun:test";
import { serve, type Server } from "bun";
import { FRAME_SNAPSHOT, decodeFrame, type ClientMessage, type ServerMessage } from "../protocol";
import { bindTerminalServer, isAllowedUpgrade, terminalSocket, upgradeTerminalSocket, type SocketData } from "./socket";
import { aliveCount, rawHttp, waitFor } from "../../test/helpers";
import { closeTerminal, createTerminal, getTerminal, shutdownTerminals, writeInput } from "./terminals";

const decoder = new TextDecoder();

let server: Server<SocketData>;

beforeAll(() => {
  server = serve<SocketData>({
    port: 0,
    hostname: "localhost",
    fetch: (req, server) => upgradeTerminalSocket(req, server) ?? new Response(null),
    websocket: terminalSocket,
  });
  bindTerminalServer(server);
});

afterEach(async () => {
  await shutdownTerminals();
});

afterAll(async () => {
  await server.stop(true);
});

class Client {
  ws: WebSocket;
  messages: ServerMessage[] = [];
  snapshots: Record<string, string[]> = {};
  output: Record<string, string> = {};
  private decoders: Record<string, TextDecoder> = {};

  private constructor(ws: WebSocket) {
    this.ws = ws;
    ws.binaryType = "arraybuffer";
    ws.onmessage = event => {
      if (typeof event.data === "string") {
        this.messages.push(JSON.parse(event.data));
        return;
      }
      const { kind, id, payload } = decodeFrame(event.data);
      if (kind === FRAME_SNAPSHOT) (this.snapshots[id] ??= []).push(decoder.decode(payload));
      else {
        const streamDecoder = (this.decoders[id] ??= new TextDecoder());
        this.output[id] = (this.output[id] ?? "") + streamDecoder.decode(payload, { stream: true });
      }
    };
  }

  static async connect() {
    const ws = new WebSocket(`ws://localhost:${server.port}/pty`, {
      headers: { origin: `http://localhost:${server.port}` },
    } as never);
    const client = new Client(ws);
    await new Promise<void>((resolve, reject) => {
      ws.onopen = () => resolve();
      ws.onerror = () => reject(new Error("WebSocket failed to open"));
    });
    return client;
  }

  send(message: ClientMessage) {
    this.ws.send(JSON.stringify(message));
  }

  close() {
    this.ws.close();
  }

  /** Waits for the snapshot a given attach produces. */
  async attach(id: string, size = { cols: 80, rows: 24 }) {
    const before = this.snapshots[id]?.length ?? 0;
    this.send({ t: "attach", id, ...size });
    await waitFor(() => (this.snapshots[id]?.length ?? 0) > before);
    return this.snapshots[id]!.at(-1)!;
  }

  status() {
    return this.messages.filter((m): m is Extract<ServerMessage, { t: "status" }> => m.t === "status");
  }
}

const sh = ["/bin/sh"];

describe("connection", () => {
  const port = 3101;
  const request = (headers: Record<string, string>) => new Request(`http://localhost:${port}/pty`, { headers });
  const allowed = (headers: Record<string, string>, peer: string | undefined = "127.0.0.1") =>
    isAllowedUpgrade(request(headers), port, peer);

  test("the upgrade check accepts a loopback Host with exactly the matching http Origin", () => {
    expect(allowed({ host: `localhost:${port}`, origin: `http://localhost:${port}` })).toBe(true);
    expect(allowed({ host: `127.0.0.1:${port}`, origin: `http://127.0.0.1:${port}` })).toBe(true);
    expect(allowed({ host: `[::1]:${port}`, origin: `http://[::1]:${port}` }, "::1")).toBe(true);
  });

  test("the upgrade check rejects rebound or malformed Hosts, any other Origin, and peers that aren't this machine", () => {
    const host = `localhost:${port}`;
    const rejected: Record<string, string>[] = [
      // A rebound Host, even when Origin agrees with it.
      { host: `evil.example:${port}`, origin: `http://evil.example:${port}` },
      { host: `localhost.evil.example:${port}`, origin: `http://localhost.evil.example:${port}` },
      { host: `127.0.0.1.evil.example:${port}`, origin: `http://127.0.0.1.evil.example:${port}` },
      // An Origin that differs in scheme, host, port or spelling.
      { host, origin: `https://localhost:${port}` },
      { host, origin: `http://127.0.0.1:${port}` },
      { host, origin: `http://localhost:${port + 1}` },
      { host, origin: "http://localhost" },
      { host, origin: `http://localhost:${port}/` },
      { host, origin: "null" },
      { host },
      // A loopback Host on a different port.
      { host: `localhost:${port + 1}`, origin: `http://localhost:${port + 1}` },
      { host: "localhost", origin: "http://localhost" },
      // A missing or malformed Host.
      { origin: `http://localhost:${port}` },
      { host: "not a host", origin: "http://not a host" },
      { host: `localhost:${port}@evil.example`, origin: `http://localhost:${port}@evil.example` },
    ];
    for (const headers of rejected) expect(allowed(headers)).toBe(false);

    const legitimate = { host, origin: `http://localhost:${port}` };
    expect(allowed(legitimate, "192.168.1.20")).toBe(false);
    expect(isAllowedUpgrade(request(legitimate), port, undefined)).toBe(false);
  });

  test("the running server upgrades a legitimate request and refuses a missing, cross-site or rebound one", async () => {
    const upgrade = (host: string, origin?: string) =>
      rawHttp(server.port!, [
        "GET /pty HTTP/1.1",
        `Host: ${host}`,
        ...(origin ? [`Origin: ${origin}`] : []),
        "Upgrade: websocket",
        "Connection: Upgrade",
        "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==",
        "Sec-WebSocket-Version: 13",
      ]);
    const own = `localhost:${server.port}`;
    expect(await upgrade(own, `http://${own}`)).toContain("101");

    expect(await upgrade(own)).toContain("403");
    expect(await upgrade(own, "http://evil.example")).toContain("403");
    expect(await upgrade(own, "not a url")).toContain("403");
    expect(await upgrade(own, `https://${own}`)).toContain("403");
    const rebound = `evil.example:${server.port}`;
    expect(await upgrade(rebound, `http://${rebound}`)).toContain("403");
  });

  test("a client can't spawn or close a terminal", async () => {
    const client = await Client.connect();
    client.send({ t: "attach", id: "nope", cols: 80, rows: 24, create: {} } as never);
    await waitFor(() => client.messages.some(m => m.t === "gone"));
    expect(getTerminal("nope")).toBeUndefined();

    const entry = createTerminal({ id: "kept", cols: 80, rows: 24, command: sh });
    await client.attach("kept");
    client.send({ t: "close", id: "kept" } as never);
    client.send({ t: "input", id: "kept", data: "echo still-$((1+1))\n" });
    await waitFor(() => client.output.kept?.includes("still-2") ?? false);
    expect(getTerminal("kept")).toBe(entry);
    client.close();
  });
});

describe("attach and detach", () => {
  test("attach sends a snapshot of what already happened, then live output to every attached client", async () => {
    const entry = createTerminal({ id: "t", cols: 80, rows: 24, command: sh });
    writeInput("t", "echo before-attach\n");
    await waitFor(() => entry.lastOutputAt !== null);
    await Bun.sleep(100);

    const a = await Client.connect();
    const b = await Client.connect();
    expect(await a.attach("t")).toContain("before-attach");
    await b.attach("t");

    a.send({ t: "input", id: "t", data: "echo after-attach\n" });
    await waitFor(() => (a.output.t?.includes("after-attach") && b.output.t?.includes("after-attach")) ?? false);
    expect(a.output.t).not.toContain("before-attach\r\n$");
    a.close();
    b.close();
  });

  test("a detached client receives no output, and a re-attach resumes with a fresh snapshot", async () => {
    createTerminal({ id: "t", cols: 80, rows: 24, command: sh });
    const client = await Client.connect();
    await client.attach("t");

    client.send({ t: "detach", id: "t" });
    // Detach and the input below arrive on the same socket, so they are processed in order.
    client.send({ t: "input", id: "t", data: "echo while-detached\n" });
    await waitFor(() => (getTerminal("t")?.lastOutputAt ?? 0) > 0);
    await Bun.sleep(150);
    expect(client.output.t ?? "").not.toContain("while-detached");

    const snapshot = await client.attach("t");
    expect(snapshot).toContain("while-detached");
    client.close();
  });
});

describe("persistence", () => {
  test("terminals survive their socket closing, and a new socket reattaches with the screen restored", async () => {
    const first = await Client.connect();
    createTerminal({ id: "t", cols: 80, rows: 24, command: sh });
    await first.attach("t");
    first.send({ t: "input", id: "t", data: "echo survives-reload\n" });
    await waitFor(() => first.output.t?.includes("survives-reload") ?? false);

    first.close();
    await Bun.sleep(100);
    const entry = getTerminal("t")!;
    expect(entry.state).toBe("running");
    expect(entry.proc.killed).toBe(false);

    const second = await Client.connect();
    const snapshot = await second.attach("t");
    expect(snapshot).toContain("survives-reload");

    second.send({ t: "input", id: "t", data: "echo still-alive\n" });
    await waitFor(() => second.output.t?.includes("still-alive") ?? false);
    second.close();
  });

  test("an exited terminal's exit reaches clients, and it is forgotten once what its shell left behind is killed", async () => {
    const client = await Client.connect();
    const entry = createTerminal({ id: "t", cols: 80, rows: 24, command: ["/bin/sh", "-c", "nohup sleep 7203 >/dev/null 2>&1 & echo bye; exit 3"] });
    await client.attach("t");
    await waitFor(() => client.messages.some(m => m.t === "exit"));
    expect(client.messages.find(m => m.t === "exit")).toEqual({ t: "exit", id: "t", exitCode: 3 });
    await waitFor(() => getTerminal("t") === undefined);
    await entry.reaping;
    expect(aliveCount("sleep 7203")).toBe(0);
    await waitFor(() => client.status().some(m => m.removed?.includes("t")));

    client.send({ t: "attach", id: "t", cols: 80, rows: 24 });
    await waitFor(() => client.messages.some(m => m.t === "gone"));
    expect(await closeTerminal("t")).toBe(false);
    client.close();
  });
});

describe("status stream", () => {
  test("a new client gets every terminal's status immediately, without attaching", async () => {
    createTerminal({ id: "one", cols: 80, rows: 24, command: sh });
    createTerminal({ id: "two", cols: 80, rows: 24, command: sh });

    const client = await Client.connect();
    await waitFor(() => client.status().length > 0);
    const first = client.status()[0]!;
    expect(first.full).toBe(true);
    expect(first.terminals.one).toMatchObject({ state: "running", exitCode: null });
    expect(first.terminals.two).toMatchObject({ state: "running", exitCode: null });
    client.close();
  });

  test("changes reach every client, throttled, without output bytes", async () => {
    const watcher = await Client.connect();
    const bystander = await Client.connect();
    createTerminal({ id: "busy", cols: 80, rows: 24, command: sh });
    await watcher.attach("busy");

    const startedAt = Date.now();
    writeInput("busy", "i=0; while [ $i -lt 400 ]; do echo flood-$i; i=$((i+1)); done\n");
    await waitFor(() => watcher.output.busy?.includes("flood-399") ?? false);
    await Bun.sleep(300);
    const elapsed = Date.now() - startedAt;

    for (const client of [watcher, bystander]) {
      const updates = client.status().filter(m => m.terminals.busy);
      expect(updates.length).toBeGreaterThan(0);
      // One update per 250ms at most, however much output there was.
      expect(updates.length).toBeLessThanOrEqual(Math.ceil(elapsed / 250) + 1);
      expect(updates.at(-1)!.terminals.busy!.lastOutputAt).toBeGreaterThanOrEqual(startedAt);
      expect(JSON.stringify(updates)).not.toContain("flood-");
    }
    // Only attached clients get a terminal's output.
    expect(bystander.output.busy).toBeUndefined();
    watcher.close();
    bystander.close();
  });

  test("closing a terminal is reported as its removal", async () => {
    createTerminal({ id: "t", cols: 80, rows: 24, command: sh });
    const client = await Client.connect();
    await client.attach("t");
    await closeTerminal("t");
    await waitFor(() => client.status().some(m => m.removed?.includes("t")));
    client.close();
  });
});
