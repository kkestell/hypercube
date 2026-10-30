import { mkdir, rm } from "node:fs/promises";
import { dirname, isAbsolute, join, relative } from "node:path";
import { homedir } from "node:os";
import type { Session } from "../model";
import { DOCKER_CLONE_PATH, dockerEnvironment } from "./docker";
import { spawnLocalPty, type PtyOptions, type PtyProcess } from "./localPty";

/** What every process the server starts offers, piped or on a PTY. Whoever started one owns it until `stop()` settles. */
export type Stoppable = {
  /** Stops the process and everything it owns, gracefully (SIGTERM, then SIGKILL after a grace period), and resolves once they are gone. Calling it again returns the same promise. */
  stop(): Promise<void>;
  /** Kills the same set with SIGKILL, synchronously, for when the server process is exiting. */
  stopNow(): void;
};

export type EnvironmentProcess = Stoppable & {
  stdin: Bun.FileSink;
  stdout: ReadableStream<Uint8Array>;
  stderr: ReadableStream<Uint8Array>;
  exited: Promise<number>;
};

const STOP_GRACE_MS = 5000;

export interface Environment {
  setup(repo: string, clonePath: string): Promise<void>;
  /** Makes sure what the session runs in is up, starting it if it was stopped. Throws, saying why, if it can't be. */
  ensureRunning(): Promise<void>;
  spawn(cmd: string[], cwd: string, env?: Record<string, string | undefined>): EnvironmentProcess;
  run(cmd: string[], cwd?: string): Promise<{ stdout: string; stderr: string }>;
  /** Runs `cmd` on a PTY. The PTY owns every process it starts; see `PtyProcess`. Throws if it can't be started. */
  spawnPty(cmd: string[], cwd: string, options: PtyOptions): PtyProcess;
  /** The command for an interactive login shell in this environment. */
  loginShell(): string[];
  teardown(clonePath: string): Promise<void>;
  /** `url`, a URL of this server, as a process in this environment reaches it. */
  hostUrl(url: string): string;
}

export function localClonePath(sessionId: string) {
  return join(homedir(), ".local/share/hypercube/sessions", sessionId);
}

/** Where the session's clone is, in its environment. */
export function clonePathFor(environment: Session["environment"], sessionId: string) {
  return environment === "docker" ? DOCKER_CLONE_PATH : localClonePath(sessionId);
}

/** Waits for `child` and returns its output, or throws what it said if it failed. `cmd` is for the error message. */
export async function collect(child: EnvironmentProcess, cmd: string[]) {
  child.stdin.end();
  const [stdout, stderr, code] = await Promise.all([new Response(child.stdout).text(), new Response(child.stderr).text(), child.exited]);
  if (code !== 0) throw new Error(stderr.trim() || stdout.trim() || `${cmd.join(" ")} exited with code ${code}`);
  return { stdout, stderr };
}

/** The real path of the existing directory `dir` names, relative to the clone or absolute. Throws unless it is inside the clone. */
export async function resolveDir(env: Environment, clonePath: string, dir: string) {
  // Real paths (`pwd -P`), so a symlink in the clone can't lead out of it.
  let root: string;
  let target: string;
  try {
    [root, target] = (await env.run(["sh", "-c", 'cd "$0" && pwd -P && cd "$1" && pwd -P', clonePath, dir], clonePath)).stdout.trim().split("\n") as [string, string];
  } catch {
    throw new Error(`"${dir}" is not a directory that exists`);
  }
  const inside = relative(root, target);
  if (inside === ".." || inside.startsWith("../") || isAbsolute(inside)) throw new Error(`"${dir}" is outside the session's clone`);
  return target;
}

export const localEnvironment: Environment = {
  async setup(repo, clonePath) {
    await mkdir(dirname(clonePath), { recursive: true });
    await localEnvironment.run(["gh", "repo", "clone", repo, clonePath]);
  },

  spawn(cmd, cwd, env) {
    const child = Bun.spawn(cmd, {
      cwd,
      env: { ...process.env, ...env },
      detached: true,
      stdin: "pipe",
      stdout: "pipe",
      stderr: "pipe",
    });
    // Only while the leader is alive: once it is gone the group id may be someone else's.
    const signal = (name: NodeJS.Signals) => {
      if (child.exitCode !== null || child.signalCode !== null) return;
      try {
        process.kill(-child.pid, name);
      } catch (error) {
        if ((error as NodeJS.ErrnoException).code !== "ESRCH") throw error;
      }
    };
    let stopping: Promise<void> | undefined;
    const stop = async () => {
      signal("SIGTERM");
      await Promise.race([child.exited, Bun.sleep(STOP_GRACE_MS)]);
      signal("SIGKILL");
      await child.exited;
    };
    return {
      stdin: child.stdin,
      stdout: child.stdout,
      stderr: child.stderr,
      exited: child.exited,
      stop: () => (stopping ??= stop()),
      stopNow: () => signal("SIGKILL"),
    };
  },

  ensureRunning: async () => {},

  run: (cmd, cwd = process.cwd()) => collect(localEnvironment.spawn(cmd, cwd), cmd),

  spawnPty: (cmd, cwd, options) => spawnLocalPty(cmd, cwd, options),

  loginShell: () => [process.env.SHELL ?? "/bin/zsh", "-l"],

  async teardown(clonePath) {
    await rm(clonePath, { recursive: true, force: true });
  },

  hostUrl: url => url,
};

/** The environment the session's clone and processes are in. */
export function environmentFor(session: Pick<Session, "id" | "environment">): Environment {
  return session.environment === "docker" ? dockerEnvironment(session.id) : localEnvironment;
}
