import { dlopen, FFIType } from "bun:ffi";

/**
 * A PTY on this machine, and what it owns. The terminal registry
 * (`terminals.ts`) works with `PtyProcess` and doesn't know how a PTY is
 * made, so another environment can provide its own.
 */

export type PtyProcess = {
  /** The PTY's session id, which is the shell's pid. Every process the terminal started is in this session. */
  sid: number;
  write(data: string | Uint8Array): void;
  resize(cols: number, rows: number): void;
  /** Closes the PTY, which makes the kernel hang up on its foreground job. */
  hangup(): void;
  /** Resolves when the shell has exited. */
  exited: Promise<void>;
  /** Stops everything the terminal started and resolves once it is gone. Calling it again is harmless. */
  reap(): Promise<void>;
  /** The same, synchronously and with SIGKILL, for when the server process is exiting. */
  killNow(): void;
};

export type PtyOptions = {
  cols: number;
  rows: number;
  /** Output from the PTY. The buffer is not reused. */
  onData(data: Uint8Array): void;
};

const KILL_GRACE_MS = 2000;
const KILL_POLL_MS = 100;

/**
 * Markers a Claude Code session sets on its child processes. If the server was
 * started from inside Claude Code, terminals must not inherit them: an agent
 * that sees them thinks it is a child session (and, for one, stops saving
 * transcripts, which breaks resume).
 */
const PARENT_SESSION_ENV = [
  "CLAUDECODE",
  "CLAUDE_PID",
  "CLAUDE_EFFORT",
  "CLAUDE_CODE_ENTRYPOINT",
  "CLAUDE_CODE_EXECPATH",
  "CLAUDE_CODE_SESSION_ID",
  "CLAUDE_CODE_CHILD_SESSION",
  "CLAUDE_CODE_SESSION_ATTENDED",
  "CLAUDE_CODE_MESSAGING_SOCKET",
  "CLAUDE_CODE_MESSAGING_TOKEN",
];

function terminalEnv(extra: Record<string, string> | undefined) {
  const env: Record<string, string | undefined> = { ...process.env, ...extra, TERM: "xterm-256color", COLORTERM: "truecolor" };
  for (const name of PARENT_SESSION_ENV) delete env[name];
  return env;
}

/** Spawns `cmd` on a new PTY in `cwd`. Throws if it can't be started (missing binary, missing cwd). */
export function spawnLocalPty(cmd: string[], cwd: string, { cols, rows, onData }: PtyOptions, env?: Record<string, string>): PtyProcess {
  const proc = Bun.spawn(cmd, {
    cwd,
    env: terminalEnv(env),
    // Copy: the buffer may be reused after the callback returns, and output is batched.
    terminal: { cols, rows, data: (_terminal, data) => onData(data.slice()) },
  });
  /** The session was found empty. Its id may now belong to someone else, so it is never looked up again. */
  let reaped = false;
  let reaping: Promise<void> | null = null;

  /**
   * Live processes in the terminal's session, wherever they were reparented to,
   * out of `pids`. The kernel does not reuse a session id while the session has
   * members, so a member found while the session has never been seen empty
   * really is ours. Once it is empty, the id is never looked up again. A live
   * process holding the shell's pid after the shell exited also means the id
   * has been reused.
   */
  const membersOf = (pids: number[]) => {
    if (reaped) return [];
    const shellExited = proc.exitCode !== null || proc.signalCode !== null;
    const members =
      shellExited && pids.includes(pty.sid)
        ? []
        : pids.filter(pid => pid !== process.pid && pid > 1 && libc.symbols.getsid(pid) === pty.sid);
    if (shellExited && members.length === 0) reaped = true;
    return members;
  };

  /** The session's members, or null if the process list couldn't be read. */
  const scanMembers = async () => {
    if (reaped) return [];
    const pids = await allPids();
    return pids && membersOf(pids);
  };

  /** Signals the members and returns how many there were; null if the scan failed, in which case nothing is signalled. */
  const signalMembers = async (...signals: NodeJS.Signals[]) => {
    const pids = await scanMembers();
    for (const pid of pids ?? []) for (const signal of signals) signalPid(pid, signal);
    return pids?.length ?? null;
  };

  /**
   * An interactive shell ignores SIGTERM and runs each job in its own process
   * group, and `nohup`ed jobs outlive it, so this signals the whole session:
   * SIGTERM and SIGHUP, then SIGKILL for whatever is left after a grace
   * period. Processes that left the session with setsid are no longer the
   * terminal's and are not touched.
   */
  const reap = async () => {
    if (!(await signalMembers("SIGTERM", "SIGHUP"))) return;
    const deadline = Date.now() + KILL_GRACE_MS;
    while (Date.now() < deadline && (await scanMembers())?.length !== 0) await Bun.sleep(KILL_POLL_MS);
    for (let attempts = 0; attempts < 50; attempts++) {
      const left = await signalMembers("SIGKILL");
      if (left === 0) return;
      await Bun.sleep(20);
    }
  };

  const pty: PtyProcess = {
    sid: proc.pid,
    write: data => proc.terminal?.write(data),
    resize: (cols, rows) => proc.terminal?.resize(cols, rows),
    hangup: () => proc.terminal?.close(),
    exited: proc.exited.then(() => {}),
    reap: () => (reaping ??= reap()),
    killNow() {
      if (reaped) return;
      for (const pid of membersOf(parsePids(Bun.spawnSync(PS).stdout.toString()))) signalPid(pid, "SIGKILL");
    },
  };
  return pty;
}

const libc = dlopen(process.platform === "darwin" ? "libSystem.B.dylib" : "libc.so.6", {
  getsid: { args: [FFIType.i32], returns: FFIType.i32 },
});

const PS = ["ps", "-axo", "pid="];
const PS_TIMEOUT_MS = 2000;
const PS_ATTEMPTS = 3;

const parsePids = (text: string) => text.split("\n").map(Number).filter(Boolean);

/**
 * Every live pid, without blocking the event loop. `Bun.spawnSync` has been
 * seen to never return here (its `ps` left unreaped), so this runs `ps`
 * asynchronously, kills it if it takes too long, and retries. Returns null if
 * no attempt succeeded.
 */
async function allPids() {
  for (let attempt = 0; attempt < PS_ATTEMPTS; attempt++) {
    const ps = Bun.spawn(PS, { stdout: "pipe", stderr: "ignore" });
    // Our own unreaped child, so its pid can't have been reused.
    const timer = setTimeout(() => ps.kill("SIGKILL"), PS_TIMEOUT_MS);
    try {
      const [text, code] = await Promise.all([new Response(ps.stdout).text(), ps.exited]);
      if (code === 0) return parsePids(text);
    } catch {
      // Try again.
    } finally {
      clearTimeout(timer);
    }
  }
  return null;
}

function signalPid(pid: number, signal: NodeJS.Signals) {
  try {
    process.kill(pid, signal);
  } catch {
    // Already gone.
  }
}
