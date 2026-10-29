import { mkdir, realpath, rm, stat } from "node:fs/promises";
import { dirname, isAbsolute, join, relative, resolve } from "node:path";
import { homedir } from "node:os";
import type { Session } from "../model";
import { DOCKER_CLONE_PATH, dockerEnvironment } from "./docker";
import { spawnLocalPty, type PtyOptions, type PtyProcess } from "./localPty";

export type EnvironmentProcess = {
  pid: number;
  stdin: Bun.FileSink;
  stdout: ReadableStream<Uint8Array>;
  stderr: ReadableStream<Uint8Array>;
  exited: Promise<number>;
  kill: () => void;
  /** Stops it and everything it owns, synchronously, for when the server process is exiting. */
  killNow: () => void;
};

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
  /** The existing directory `dir` names, relative to the clone or absolute, if it is inside the clone. Throws otherwise. */
  resolveDir(clonePath: string, dir: string): Promise<string>;
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

/** Throws unless `target` is `root` or inside it. Both are real paths. `dir` is what the caller named, for the error. */
export function requireInside(root: string, target: string, dir: string) {
  const inside = relative(root, target);
  if (inside === ".." || inside.startsWith("../") || isAbsolute(inside)) throw new Error(`"${dir}" is outside the session's clone`);
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
    const kill = () => {
      if (child.exitCode !== null) return;
      try {
        process.kill(-child.pid, "SIGTERM");
      } catch (error) {
        if ((error as NodeJS.ErrnoException).code !== "ESRCH") throw error;
      }
    };
    return { pid: child.pid, stdin: child.stdin, stdout: child.stdout, stderr: child.stderr, exited: child.exited, kill, killNow: kill };
  },

  ensureRunning: async () => {},

  run: (cmd, cwd = process.cwd()) => collect(localEnvironment.spawn(cmd, cwd), cmd),

  spawnPty: (cmd, cwd, options) => spawnLocalPty(cmd, cwd, options),

  loginShell: () => [process.env.SHELL ?? "/bin/zsh", "-l"],

  async teardown(clonePath) {
    await rm(clonePath, { recursive: true, force: true });
  },

  async resolveDir(clonePath, dir) {
    // Real paths, so a symlink in the clone can't lead out of it.
    let root: string;
    let target: string;
    try {
      root = await realpath(clonePath);
      target = await realpath(resolve(clonePath, dir));
      if (!(await stat(target)).isDirectory()) throw new Error("not a directory");
    } catch {
      throw new Error(`"${dir}" is not a directory that exists`);
    }
    requireInside(root, target, dir);
    return target;
  },
};

/** The environment the session's clone and processes are in. */
export function environmentFor(session: Pick<Session, "id" | "environment">): Environment {
  return session.environment === "docker" ? dockerEnvironment(session.id) : localEnvironment;
}
