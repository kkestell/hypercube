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
  test.serial("adds a GitHub repo by its normalized name, normalized", async () => {
    const id = await addWorkspace(" https://github.com/owner/repo.git/ ");
    const ws = model().workspaces.find(ws => ws.id === id)!;
    expect(ws.repo).toBe("owner/repo");
    expect(run).toHaveBeenCalledWith(["gh", "repo", "view", "owner/repo", "--json", "nameWithOwner"]);
  });

  test.serial("accepts an SSH GitHub URL and rejects invalid or unavailable repos", async () => {
    await expect(addWorkspace("~/projects/repo")).rejects.toThrow("GitHub repository");
    await expect(addWorkspace("https://example.com/owner/repo")).rejects.toThrow("GitHub repository");
    run.mockRejectedValueOnce(new Error("Could not resolve to a Repository"));
    await expect(addWorkspace("owner/missing")).rejects.toThrow("Could not resolve to a Repository");
    expect(model().workspaces).toEqual([]);
    const id = await addWorkspace("git@github.com:owner/repo.git");
    expect(model().workspaces.find(ws => ws.id === id)!.repo).toBe("owner/repo");
  });

  test.serial("rejects a repo that is already open by name or URL", async () => {
    await addWorkspace("owner/repo");
    await expect(addWorkspace("git@github.com:owner/repo.git")).rejects.toThrow("already open");
    await expect(addWorkspace("https://github.com/owner/repo/")).rejects.toThrow("already open");
    expect(model().workspaces).toHaveLength(1);
  });
});

describe("sessions", () => {
  test.serial("a session is Untitled N, counting up per workspace, with a lead agent", async () => {
    const first = await addWorkspace("owner/first");
    const second = await addWorkspace("owner/second");
    const names = (workspaceId: string) => Object.values(model().sessions).filter(session => session.workspaceId === workspaceId).map(session => session.name);

    createSession(first, "local");
    createSession(first, "local");
    createSession(second, "local");
    expect(names(first)).toEqual(["Untitled 1", "Untitled 2"]);
    expect(names(second)).toEqual(["Untitled 1"]);

    const session = Object.values(model().sessions)[0]!;
    expect(session).toMatchObject({ environment: "local", clonePath: localClonePath(session.id), status: "cloning" });
    expect(setup).toHaveBeenCalledWith("owner/first", session.clonePath);
    const agents = Object.values(model().agents).filter(agent => agent.sessionId === session.id);
    expect(agents).toHaveLength(1);
    expect(agents[0]).toMatchObject({ role: "lead", name: "Lead" });
  });

  test.serial("a session in a workspace that doesn't exist is refused", () => {
    expect(() => createSession("nope", "local")).toThrow("No such workspace");
    expect(model().sessions).toEqual({});
  });
});
