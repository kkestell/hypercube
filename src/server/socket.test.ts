import { afterAll, beforeAll, beforeEach, describe, expect, test } from "bun:test";
import { mkdtempSync, realpathSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { serve, type Server } from "bun";
import type { ServerMessage } from "../protocol";
import { bindServer, isAllowedUpgrade, socket, upgradeSocket, type SocketData } from "./socket";
import { getModel, resetModelForTests } from "./sessions";
import { rawHttp, waitFor } from "../../test/helpers";

let server: Server<SocketData>;

beforeAll(() => {
  server = serve<SocketData>({
    port: 0,
    hostname: "localhost",
    fetch: (req, server) => upgradeSocket(req, server) ?? new Response(null),
    websocket: socket,
  });
  bindServer(server);
});

beforeEach(resetModelForTests);

afterAll(async () => {
  await server.stop(true);
});

describe("connection", () => {
  const port = 3101;
  const request = (headers: Record<string, string>) => new Request(`http://localhost:${port}/ws`, { headers });
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
        "GET /ws HTTP/1.1",
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
});

describe("model", () => {
  class Client {
    messages: ServerMessage[] = [];
    private constructor(readonly ws: WebSocket) {
      ws.onmessage = event => this.messages.push(JSON.parse(event.data));
    }
    static async connect() {
      const ws = new WebSocket(`ws://localhost:${server.port}/ws`, { headers: { origin: `http://localhost:${server.port}` } } as never);
      await new Promise(resolve => (ws.onopen = resolve));
      return new Client(ws);
    }
    send(message: unknown) {
      this.ws.send(JSON.stringify(message));
    }
    models() {
      return this.messages.filter((m): m is Extract<ServerMessage, { t: "model" }> => m.t === "model");
    }
  }

  test("a client gets the model on connect, and a change's model before the change's result", async () => {
    const dir = realpathSync(mkdtempSync(join(tmpdir(), "hypercube-ws-")));
    const client = await Client.connect();
    await waitFor(() => client.models().length > 0);

    client.send({ t: "addWorkspace", req: 7, path: dir });
    await waitFor(() => client.messages.some(m => m.t === "result"));
    const result = client.messages.findIndex(m => m.t === "result");
    const withWorkspace = client.messages.findIndex(m => m.t === "model" && m.model.workspaces.length === 1);
    expect(withWorkspace).toBeGreaterThan(-1);
    expect(withWorkspace).toBeLessThan(result);
    expect(client.messages[result]).toMatchObject({ t: "result", req: 7, ok: true, id: getModel().workspaces[0]!.id });

    client.send({ t: "addWorkspace", req: 8, path: dir });
    await waitFor(() => client.messages.some(m => m.t === "result" && m.req === 8));
    expect(client.messages.find(m => m.t === "result" && m.req === 8)).toMatchObject({ ok: false, error: expect.stringContaining("already open") });

    const again = await Client.connect();
    await waitFor(() => again.models().some(m => m.model.workspaces.length === 1));
    client.ws.close();
    again.ws.close();
  });

  test("a change reaches every connected client", async () => {
    const dir = realpathSync(mkdtempSync(join(tmpdir(), "hypercube-ws-")));
    const [a, b] = await Promise.all([Client.connect(), Client.connect()]);
    a.send({ t: "addWorkspace", req: 1, path: dir });
    await waitFor(() => a.messages.some(m => m.t === "result"));
    const workspaceId = getModel().workspaces[0]!.id;

    a.send({ t: "createSession", req: 2, workspaceId });
    await waitFor(() => Object.keys(b.models().at(-1)!.model.sessions).length === 1);
    expect(Object.values(b.models().at(-1)!.model.sessions)[0]!.name).toBe("Untitled 1");
    a.ws.close();
    b.ws.close();
  });
});
