import { beforeEach, describe, expect, test } from "bun:test";
import { mkdtempSync, realpathSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { addWorkspace, createSession, getModel, resetModelForTests } from "./sessions";

let dir: string;

beforeEach(() => {
  resetModelForTests();
  dir = realpathSync(mkdtempSync(join(tmpdir(), "hypercube-ws-")));
});

const model = () => getModel();

describe("workspaces", () => {
  test("adds a directory by its real path, named after it", () => {
    const viaTmp = mkdtempSync("/tmp/hypercube-ws-");
    const id = addWorkspace(viaTmp);
    const ws = model().workspaces.find(ws => ws.id === id)!;
    expect(ws.path).toBe(realpathSync(viaTmp));
    expect(ws.name).toBe(realpathSync(viaTmp).split("/").at(-1)!);
  });

  test("expands ~ and rejects what isn't an open-able directory", () => {
    const file = join(dir, "file");
    writeFileSync(file, "");
    expect(() => addWorkspace(join(dir, "missing"))).toThrow("does not exist");
    expect(() => addWorkspace(file)).toThrow("not a directory");
    expect(() => addWorkspace("relative/path")).toThrow("absolute");
    expect(model().workspaces).toEqual([]);
    const home = addWorkspace("~");
    expect(model().workspaces.find(ws => ws.id === home)!.path).toBe(realpathSync(process.env.HOME!));
  });

  test("rejects a directory that is already open, however it is spelled", () => {
    addWorkspace(dir);
    const link = `${dir}-link`;
    symlinkSync(dir, link);
    expect(() => addWorkspace(link)).toThrow("already open");
    expect(() => addWorkspace(`${dir}/`)).toThrow("already open");
    expect(model().workspaces).toHaveLength(1);
  });
});

describe("sessions", () => {
  test("a session is Untitled N, counting up per workspace, with a lead agent", () => {
    const first = addWorkspace(dir);
    const second = addWorkspace(mkdtempSync(join(tmpdir(), "hypercube-ws-")));
    const names = (workspaceId: string) => model().workspaces.find(ws => ws.id === workspaceId)!.sessionIds.map(id => model().sessions[id]!.name);

    createSession(first);
    createSession(first);
    createSession(second);
    expect(names(first)).toEqual(["Untitled 1", "Untitled 2"]);
    expect(names(second)).toEqual(["Untitled 1"]);

    const session = model().sessions[model().workspaces[0]!.sessionIds[0]!]!;
    expect(session.agentIds).toHaveLength(1);
    expect(model().agents[session.agentIds[0]!]).toMatchObject({ role: "lead", name: "Lead", sessionId: session.id });
  });

  test("a session in a workspace that doesn't exist is refused", () => {
    expect(() => createSession("nope")).toThrow("No such workspace");
    expect(model().sessions).toEqual({});
  });
});
