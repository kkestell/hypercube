import { afterAll, beforeEach, describe, expect, spyOn, test } from "bun:test";
import { localClonePath, localEnvironment } from "./environment";
import { addWorkspace, createSession, getModel, resetModelForTests } from "./sessions";

const run = spyOn(localEnvironment, "run");
const setup = spyOn(localEnvironment, "setup");
// A session that becomes ready starts its lead; no test runs a real process.
const spawn = spyOn(localEnvironment, "spawn");

beforeEach(() => {
  resetModelForTests();
  run.mockReset().mockResolvedValue({ stdout: "", stderr: "" });
  setup.mockReset().mockResolvedValue(undefined);
  spawn.mockReset().mockImplementation(() => {
    throw new Error("no processes in tests");
  });
});

afterAll(() => {
  run.mockRestore();
  setup.mockRestore();
  spawn.mockRestore();
});

const model = () => getModel();

describe("workspaces", () => {
  test("adds a GitHub repo by its normalized name, named after it", async () => {
    const id = await addWorkspace(" https://github.com/owner/repo.git/ ");
    const ws = model().workspaces.find(ws => ws.id === id)!;
    expect(ws.repo).toBe("owner/repo");
    expect(ws.name).toBe("owner/repo");
    expect(run).toHaveBeenCalledWith(["gh", "repo", "view", "owner/repo", "--json", "nameWithOwner"]);
  });

  test("accepts an SSH GitHub URL and rejects invalid or unavailable repos", async () => {
    await expect(addWorkspace("~/projects/repo")).rejects.toThrow("GitHub repository");
    await expect(addWorkspace("https://example.com/owner/repo")).rejects.toThrow("GitHub repository");
    run.mockRejectedValueOnce(new Error("Could not resolve to a Repository"));
    await expect(addWorkspace("owner/missing")).rejects.toThrow("Could not resolve to a Repository");
    expect(model().workspaces).toEqual([]);
    const id = await addWorkspace("git@github.com:owner/repo.git");
    expect(model().workspaces.find(ws => ws.id === id)!.repo).toBe("owner/repo");
  });

  test("rejects a repo that is already open by name or URL", async () => {
    await addWorkspace("owner/repo");
    await expect(addWorkspace("git@github.com:owner/repo.git")).rejects.toThrow("already open");
    await expect(addWorkspace("https://github.com/owner/repo/")).rejects.toThrow("already open");
    expect(model().workspaces).toHaveLength(1);
  });
});

describe("sessions", () => {
  test("a session is Untitled N, counting up per workspace, with a lead agent", async () => {
    const first = await addWorkspace("owner/first");
    const second = await addWorkspace("owner/second");
    const names = (workspaceId: string) => model().workspaces.find(ws => ws.id === workspaceId)!.sessionIds.map(id => model().sessions[id]!.name);

    createSession(first, "local");
    createSession(first, "local");
    createSession(second, "local");
    expect(names(first)).toEqual(["Untitled 1", "Untitled 2"]);
    expect(names(second)).toEqual(["Untitled 1"]);

    const session = model().sessions[model().workspaces[0]!.sessionIds[0]!]!;
    expect(session).toMatchObject({ environment: "local", clonePath: localClonePath(session.id), status: "cloning" });
    expect(setup).toHaveBeenCalledWith("owner/first", session.clonePath);
    expect(session.agentIds).toHaveLength(1);
    expect(model().agents[session.agentIds[0]!]).toMatchObject({ role: "lead", name: "Lead", sessionId: session.id });
  });

  test("a session in a workspace that doesn't exist is refused", () => {
    expect(() => createSession("nope", "local")).toThrow("No such workspace");
    expect(model().sessions).toEqual({});
  });
});
