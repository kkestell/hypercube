import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, test } from "bun:test";
import { serve, type Server } from "bun";
import { existsSync, mkdtempSync, readFileSync, realpathSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { Agent, Model } from "../model";
import type { ServerMessage } from "../protocol";
import { AGENT_API_PATHS, handleAgentRequest } from "./agentApi";
import { harnessDir, writeHarnessFiles } from "./harness";
import {
  addWorkspace,
  agentForToken,
  createSession,
  getModel,
  resetModelForTests,
  restartShell,
  retryAgent,
  startSession,
  stopSession,
} from "./sessions";
import { aliveCount, rawHttp, waitFor } from "../../test/helpers";
import { bindTerminalServer, terminalSocket, upgradeTerminalSocket, type SocketData } from "./socket";
import { createTerminal, getTerminal, shutdownTerminals, writeInput } from "./terminals";

const fixtures = join(import.meta.dir, "../../test/fixtures");
const fakeDir = realpathSync(mkdtempSync(join(tmpdir(), "hypercube-fake-")));
process.env.FAKE_DIR = fakeDir;
const bins = { claude: join(fixtures, "fake-claude"), opencode: join(fixtures, "fake-opencode") };

let server: Server<SocketData>;
let workspaceDir: string;

beforeAll(() => {
  server = serve<SocketData>({
    port: 0,
    hostname: "localhost",
    routes: Object.fromEntries(AGENT_API_PATHS.map(path => [path, (req: Request, server: Server<SocketData>) => handleAgentRequest(req, server)])),
    fetch: (req, server) => upgradeTerminalSocket(req, server) ?? new Response(null),
    websocket: terminalSocket,
  });
  bindTerminalServer(server);
});

beforeEach(() => {
  resetModelForTests();
  process.env.HYPERCUBE_CLAUDE_BIN = bins.claude;
  process.env.HYPERCUBE_OPENCODE_BIN = bins.opencode;
  rmSync(join(fakeDir, "log.jsonl"), { force: true });
  rmSync(join(fakeDir, "crash"), { force: true });
  workspaceDir = realpathSync(mkdtempSync(join(tmpdir(), "hypercube-ws-")));
});

afterEach(async () => {
  await shutdownTerminals();
  resetModelForTests();
  Bun.spawnSync(["pkill", "-f", "sleep 7801"]);
});

afterAll(async () => {
  await server.stop(true);
});

type Launch = { harness: string; args: string[]; pid: number; cwd: string; token: string; url: string };
const launches = (): Launch[] =>
  existsSync(join(fakeDir, "log.jsonl"))
    ? readFileSync(join(fakeDir, "log.jsonl"), "utf8").trim().split("\n").filter(Boolean).map(line => JSON.parse(line))
    : [];

const model = (): Model => getModel();
const leadOf = (sessionId: string): Agent => model().agents[model().sessions[sessionId]!.agentIds[0]!]!;

/** Waits until the fake harness has started and is reading commands. */
async function ready(agent: Agent) {
  const terminalId = agent.terminalId!;
  const entry = getTerminal(terminalId)!;
  await waitFor(() => (entry.lastOutputAt ?? 0) > 0);
  await Bun.sleep(150);
  return terminalId;
}

/** A session with a running lead, ready for commands. */
async function runningSession(harness: "claude" | "opencode" = "claude") {
  const workspaceId = model().workspaces[0]?.id ?? addWorkspace(workspaceDir);
  const sessionId = createSession(workspaceId, harness);
  await ready(leadOf(sessionId));
  return { workspaceId, sessionId };
}

describe("workspaces", () => {
  test("adds a directory by its real path, named after it, with a shell started there", () => {
    const viaTmp = mkdtempSync("/tmp/hypercube-ws-");
    const id = addWorkspace(viaTmp);
    const ws = model().workspaces.find(ws => ws.id === id)!;
    expect(ws.path).toBe(realpathSync(viaTmp));
    expect(ws.name).toBe(realpathSync(viaTmp).split("/").at(-1)!);
    expect(getTerminal(ws.shell.terminalId!)!.cwd).toBe(ws.path);
  });

  test("expands ~ and rejects what isn't an open-able directory", () => {
    const file = join(workspaceDir, "file");
    writeFileSync(file, "");
    expect(() => addWorkspace(join(workspaceDir, "missing"))).toThrow("does not exist");
    expect(() => addWorkspace(file)).toThrow("not a directory");
    expect(() => addWorkspace("relative/path")).toThrow("absolute");
    expect(model().workspaces).toEqual([]);
    const home = addWorkspace("~");
    expect(model().workspaces.find(ws => ws.id === home)!.path).toBe(realpathSync(process.env.HOME!));
  });

  test("rejects a directory that is already open, however it is spelled", () => {
    addWorkspace(workspaceDir);
    const link = join(fakeDir, `link-${crypto.randomUUID()}`);
    symlinkSync(workspaceDir, link);
    expect(() => addWorkspace(link)).toThrow("already open");
    expect(() => addWorkspace(workspaceDir.replace(/^\/private\/tmp\//, "/tmp/").replace(/^\/private\/var\//, "/var/"))).toThrow("already open");
    expect(() => addWorkspace(`${workspaceDir}/`)).toThrow("already open");
    expect(model().workspaces).toHaveLength(1);
  });
});

describe("sessions", () => {
  test("creates Untitled 1, 2, … with a lead started in the workspace on a new conversation", async () => {
    const workspaceId = addWorkspace(workspaceDir);
    const first = createSession(workspaceId, "claude");
    const second = createSession(workspaceId, "opencode");
    expect([model().sessions[first]!.name, model().sessions[second]!.name]).toEqual(["Untitled 1", "Untitled 2"]);
    await waitFor(() => launches().length === 2);

    const [claude, opencode] = [leadOf(first), leadOf(second)];
    const byHarness = Object.fromEntries(launches().map(launch => [launch.harness, launch]));
    expect(byHarness.claude!.args).toEqual(["--session-id", claude.conversationId, "--settings", join(harnessDir(), "claude-settings.json")]);
    expect(byHarness.opencode!.args).toEqual(["--standalone", "--session", opencode.conversationId]);
    expect(opencode.conversationId).toMatch(/^ses_hypercube_[0-9a-f]{32}$/);
    for (const launch of launches()) expect(launch.cwd).toBe(workspaceDir);
    expect(claude).toMatchObject({ role: "lead", state: "running" });
  });

  test("a lead that exits is restarted on the same conversation, resuming once it has a transcript", async () => {
    const { sessionId } = await runningSession();
    const lead = leadOf(sessionId);
    const first = lead.terminalId!;

    writeInput(first, "exit 0\r");
    await waitFor(() => lead.state === "running" && lead.terminalId !== first && launches().length === 2);
    expect(getTerminal(first)).toBeUndefined();
    expect(launches()[1]!.args.slice(0, 2)).toEqual(["--session-id", lead.conversationId]);

    const second = await ready(lead);
    writeInput(second, "record\r");
    await waitFor(() => existsSync(join(fakeDir, `${lead.conversationId}.jsonl`)));
    writeInput(second, "exit 0\r");
    await waitFor(() => launches().length === 3);
    expect(launches()[2]!.args.slice(0, 2)).toEqual(["--resume", lead.conversationId]);
  });

  test("a lead that keeps exiting right away is marked failed, and Retry starts it again", async () => {
    const { sessionId } = await runningSession();
    const lead = leadOf(sessionId);
    writeFileSync(join(fakeDir, "crash"), "");
    writeInput(lead.terminalId!, "exit 1\r");

    await waitFor(() => lead.state === "failed", 15_000);
    const spawned = launches().length;
    expect(lead.failure).toContain("3 times");
    await Bun.sleep(700);
    expect(launches().length).toBe(spawned);

    rmSync(join(fakeDir, "crash"));
    retryAgent(lead.id);
    expect(lead.state).toBe("running");
    await ready(lead);
  }, 20_000);

  test("stop during the restart backoff cancels the restart", async () => {
    const { sessionId } = await runningSession();
    const lead = leadOf(sessionId);
    writeInput(lead.terminalId!, "exit 0\r");
    await waitFor(() => lead.state === "restarting");
    stopSession(sessionId);
    expect(lead.state).toBe("stopped");
    await Bun.sleep(700);
    expect(launches()).toHaveLength(1);
  });

  test("stop kills everything the lead started, and its exit is not a reason to restart", async () => {
    const { sessionId } = await runningSession();
    const lead = leadOf(sessionId);
    writeInput(lead.terminalId!, "child\r");
    await waitFor(() => aliveCount("sleep 7801") === 1);

    stopSession(sessionId);
    expect(lead.state).toBe("stopping");
    await waitFor(() => lead.state === "stopped");
    expect(lead.terminalId).toBeNull();
    expect(aliveCount("sleep 7801")).toBe(0);
    await Bun.sleep(700);
    expect(launches()).toHaveLength(1);
  });

  test("start while stopping starts one process, after the old one is gone", async () => {
    const { sessionId } = await runningSession();
    const lead = leadOf(sessionId);
    const old = lead.terminalId!;
    stopSession(sessionId);
    startSession(sessionId);
    expect(lead.state).toBe("stopping");
    await waitFor(() => lead.state === "running" && lead.terminalId !== old);
    expect(getTerminal(old)).toBeUndefined();
    await Bun.sleep(300);
    expect(launches()).toHaveLength(2);
  });

  test("a worker that exits is not restarted", async () => {
    const { sessionId } = await runningSession();
    const session = model().sessions[sessionId]!;
    const worker: Agent = { ...leadOf(sessionId), id: crypto.randomUUID(), role: "worker", name: "Worker", state: "stopped", terminalId: null, conversationId: crypto.randomUUID() };
    model().agents[worker.id] = worker;
    session.agentIds.push(worker.id);
    startSession(sessionId);
    const live = model().agents[worker.id]!;
    await ready(live);
    writeInput(live.terminalId!, "exit 0\r");
    await waitFor(() => live.state === "stopped");
    await Bun.sleep(700);
    expect(launches()).toHaveLength(2);
  });
});

describe("spawn failures", () => {
  test("a session whose lead can't start is not created, and uses no number", () => {
    const workspaceId = addWorkspace(workspaceDir);
    process.env.HYPERCUBE_CLAUDE_BIN = "/nonexistent/claude";
    expect(() => createSession(workspaceId, "claude")).toThrow("Could not start");
    expect(model().sessions).toEqual({});
    expect(model().agents).toEqual({});
    process.env.HYPERCUBE_CLAUDE_BIN = bins.claude;
    expect(model().sessions[createSession(workspaceId, "claude")]!.name).toBe("Untitled 1");
  });

  test("a restart that can't spawn marks the lead failed with its token revoked; Retry and Stop still work", async () => {
    const { sessionId } = await runningSession();
    const lead = leadOf(sessionId);
    const { token } = launches()[0]!;
    process.env.HYPERCUBE_CLAUDE_BIN = "/nonexistent/claude";
    writeInput(lead.terminalId!, "exit 0\r");
    await waitFor(() => lead.state === "failed");
    expect(lead.failure).toContain("Could not start");
    expect(lead.terminalId).toBeNull();
    expect(agentForToken(token)).toBeUndefined();

    process.env.HYPERCUBE_CLAUDE_BIN = bins.claude;
    retryAgent(lead.id);
    expect(lead.state).toBe("running");
    await ready(lead);
    stopSession(sessionId);
    await waitFor(() => lead.state === "stopped");
  });
});

describe("shutdown and reloads", () => {
  test("shutdown cancels a pending restart and kills everything the lead started", async () => {
    const { sessionId } = await runningSession();
    const lead = leadOf(sessionId);
    writeInput(lead.terminalId!, "child\r");
    await waitFor(() => aliveCount("sleep 7801") === 1);
    writeInput(lead.terminalId!, "exit 0\r");
    await waitFor(() => lead.state === "restarting");

    await shutdownTerminals();
    await Bun.sleep(700);
    expect(launches()).toHaveLength(1);
    expect(aliveCount("sleep 7801")).toBe(0);
    expect(aliveCount(fakeDir)).toBe(0);
  });

  test("retry timers and close completions call the current code, as after a hot reload", async () => {
    const state = (globalThis as unknown as { __hypercubeSessions: { ops: Record<string, (...args: never[]) => void> } }).__hypercubeSessions;
    const calls: string[] = [];
    const original = { ...state.ops };
    // What a reload does: replace the ops on the shared state.
    for (const name of ["retry", "closed"] as const) {
      state.ops[name] = (...args: never[]) => {
        calls.push(name);
        original[name]!(...args);
      };
    }
    try {
      const { sessionId } = await runningSession();
      const lead = leadOf(sessionId);
      writeInput(lead.terminalId!, "exit 0\r");
      await waitFor(() => calls.includes("retry"));
      await waitFor(() => lead.state === "running");
      stopSession(sessionId);
      await waitFor(() => calls.includes("closed"));
    } finally {
      Object.assign(state.ops, original);
    }
  });

  test("harness files are replaced atomically", async () => {
    const settings = join(harnessDir(), "claude-settings.json");
    const expected = readFileSync(settings, "utf8");
    let reads = 0;
    let stop = false;
    const reader = (async () => {
      while (!stop) {
        expect(readFileSync(settings, "utf8")).toBe(expected);
        reads++;
        await Bun.sleep(0);
      }
    })();
    for (let i = 0; i < 200; i++) writeHarnessFiles();
    stop = true;
    await reader;
    expect(reads).toBeGreaterThan(0);
  });
});

describe("workspace shell", () => {
  test("exit leaves it stopped with its code, and two restart requests at once start one shell", async () => {
    const workspaceId = addWorkspace(workspaceDir);
    const ws = model().workspaces[0]!;
    const first = ws.shell.terminalId!;
    await waitFor(() => (getTerminal(first)?.lastOutputAt ?? 0) > 0);
    writeInput(first, "exit 4\r");
    await waitFor(() => ws.shell.terminalId === null);
    expect(ws.shell.exitCode).toBe(4);
    await Bun.sleep(200);
    expect(ws.shell.terminalId).toBeNull();

    restartShell(workspaceId);
    const second = ws.shell.terminalId;
    restartShell(workspaceId);
    expect(ws.shell.terminalId).toBe(second);
    expect(getTerminal(second!)!.cwd).toBe(workspaceDir);
  });
});

describe("browser protocol", () => {
  class Client {
    messages: ServerMessage[] = [];
    private constructor(readonly ws: WebSocket) {
      ws.onmessage = event => typeof event.data === "string" && this.messages.push(JSON.parse(event.data));
    }
    static async connect() {
      const ws = new WebSocket(`ws://localhost:${server.port}/pty`, { headers: { origin: `http://localhost:${server.port}` } } as never);
      await new Promise(resolve => (ws.onopen = resolve));
      return new Client(ws);
    }
    send(message: unknown) {
      this.ws.send(JSON.stringify(message));
    }
  }

  test("a client gets the model on connect, and a change's model before the change's result", async () => {
    const client = await Client.connect();
    await waitFor(() => client.messages.some(m => m.t === "model"));
    client.send({ t: "addWorkspace", req: 7, path: workspaceDir });
    await waitFor(() => client.messages.some(m => m.t === "result"));
    const result = client.messages.findIndex(m => m.t === "result");
    const withWorkspace = client.messages.findIndex(m => m.t === "model" && m.model.workspaces.length === 1);
    expect(withWorkspace).toBeGreaterThan(-1);
    expect(withWorkspace).toBeLessThan(result);
    expect(client.messages[result]).toMatchObject({ t: "result", req: 7, ok: true, id: model().workspaces[0]!.id });

    client.send({ t: "addWorkspace", req: 8, path: workspaceDir });
    await waitFor(() => client.messages.some(m => m.t === "result" && m.req === 8));
    expect(client.messages.find(m => m.t === "result" && m.req === 8)).toMatchObject({ ok: false, error: expect.stringContaining("already open") });

    const again = await Client.connect();
    await waitFor(() => again.messages.some(m => m.t === "model" && m.model.workspaces.length === 1));
    client.ws.close();
    again.ws.close();
  });
});

describe("agent API", () => {
  const post = (path: string, headers: Record<string, string>, body: string) =>
    fetch(`http://localhost:${server.port}${path}`, { method: "POST", headers: { "content-type": "application/json", ...headers }, body });
  const claudeStart = "/api/agent/claude-session-start";

  test("only the live process's token, from a non-browser loopback request, is accepted", async () => {
    const { sessionId } = await runningSession();
    const lead = leadOf(sessionId);
    const { token } = launches()[0]!;
    const body = JSON.stringify({ session_id: lead.conversationId, transcript_path: "/x.jsonl", source: "startup" });
    const auth = { authorization: `Bearer ${token}` };

    expect((await post(claudeStart, auth, body)).status).toBe(204);
    expect((await post(claudeStart, {}, body)).status).toBe(403);
    expect((await post(claudeStart, { authorization: `Bearer ${"0".repeat(64)}` }, body)).status).toBe(403);
    expect((await post(claudeStart, { ...auth, origin: `http://localhost:${server.port}` }, body)).status).toBe(403);
    expect((await post(claudeStart, auth, "x".repeat(70_000))).status).toBe(413);
    expect((await post("/api/agent/opencode-route", auth, JSON.stringify({ seq: 1, route: { type: "home" } }))).status).toBe(403);

    // A request naming another host is refused, whatever its token.
    const rebound = await rawHttp(server.port!, [
      `POST ${claudeStart} HTTP/1.1`,
      `Host: evil.example:${server.port}`,
      `Authorization: Bearer ${token}`,
      "Content-Type: application/json",
      `Content-Length: ${body.length}`,
      "Connection: close",
    ], body);
    expect(rebound).toContain("403");

    // A replaced process's token stops working.
    writeInput(lead.terminalId!, "exit 0\r");
    await waitFor(() => launches().length === 2);
    expect((await post(claudeStart, auth, body)).status).toBe(403);
  });
});
