import { dlopen, FFIType } from "bun:ffi";
import type { Subprocess } from "bun";
import { Terminal as HeadlessTerminal } from "@xterm/headless";
import { SerializeAddon } from "@xterm/addon-serialize";
import { SCROLLBACK, type TerminalStatus } from "../protocol";

/**
 * Server-side terminal registry. Terminals are created by an explicit request
 * and live until explicitly closed or the server shuts down; no socket
 * disconnect ends one. Each terminal pairs a PTY process with a headless
 * xterm that mirrors its screen, so a client attaching later gets a snapshot.
 *
 * Output is published from the headless terminal's write callbacks, so it goes
 * out in the order it is parsed. Snapshots and resizes are queued the same
 * way, which puts each of them at an exact point in the output stream.
 *
 * Registry state lives on globalThis, and everything a process or the server
 * process triggers goes through `registry.ops`, so a `bun --hot` reload keeps
 * the running processes and routes them to the reloaded code.
 */

const FLUSH_INTERVAL_MS = 16;
/**
 * How long the PTY stays one row off its real size when an attach forces a
 * redraw. Apps read the size when they handle SIGWINCH, so two resizes in quick
 * succession look like no change at all; Claude Code and OpenCode both redraw
 * with a 10ms gap and neither does with none.
 */
const REDRAW_GAP_MS = 50;
const KILL_GRACE_MS = 2000;
const KILL_POLL_MS = 100;

export type TerminalEntry = {
  id: string;
  proc: Subprocess;
  /** The PTY's session id, which is the shell's pid. Every process the terminal started is in this session. */
  sid: number;
  headless: HeadlessTerminal;
  serializer: SerializeAddon;
  /** The size most recently requested. The headless terminal catches up through its write queue. */
  cols: number;
  rows: number;
  cwd: string;
  state: "running" | "exited";
  exitCode: number | null;
  lastOutputAt: number | null;
  /** Output received but not yet passed to the headless terminal. */
  pending: Uint8Array[];
  flushTimer: ReturnType<typeof setTimeout> | null;
  /** The exit event has gone out. Clients that attach after this are told directly. */
  exitPublished: boolean;
  /** Set while the PTY is off its real size to force a redraw. */
  redrawTimer: ReturnType<typeof setTimeout> | null;
  /** Killing the session's processes, started when the shell exits or the terminal is closed. */
  reaping: Promise<void> | null;
  /** The session was found empty. Its id may now belong to someone else, so it is never looked up again. */
  reaped: boolean;
};

/** Where the registry reports what happens to terminals. The socket layer provides these. */
export type Hooks = {
  /** A batch of output, already applied to the headless terminal. */
  output(entry: TerminalEntry, chunk: Uint8Array): void;
  /** Sent after the terminal's last output. */
  exit(entry: TerminalEntry): void;
  /** A terminal's status changed. */
  changed(id: string): void;
  /** A terminal was closed and forgotten. */
  removed(id: string): void;
};

/** Where the registry reports to whatever owns the terminals (the session model). */
export type OwnerHooks = {
  /** A terminal's process exited by itself, its exit has been published and nothing it started is left. It is forgotten. */
  ended(entry: TerminalEntry): void;
  /** Shutdown is starting. Nothing may be spawned from now on. */
  beforeShutdown(): void;
};

type Ops = {
  onData(entry: TerminalEntry, data: Uint8Array): void;
  onExit(entry: TerminalEntry, code: number | null): void;
  shutdown(): Promise<void>;
  /** Synchronous last resort when the server process exits. */
  killAllNow(): void;
};

type Registry = {
  terminals: Map<string, TerminalEntry>;
  /** Terminals that are no longer registered but whose processes are still being stopped. */
  closing: Set<TerminalEntry>;
  hooks: Hooks;
  owner: OwnerHooks;
  ops: Ops;
  processHandlersInstalled: boolean;
};

const noop = () => {};
const globals = globalThis as typeof globalThis & { __hypercubeTerminals?: Registry };
const registry: Registry = (globals.__hypercubeTerminals ??= {
  terminals: new Map(),
  closing: new Set(),
  hooks: { output: noop, exit: noop, changed: noop, removed: noop },
  owner: { ended: noop, beforeShutdown: noop },
  ops: undefined as unknown as Ops,
  processHandlersInstalled: false,
});

export function setHooks(hooks: Hooks) {
  registry.hooks = hooks;
}

export function setOwnerHooks(owner: OwnerHooks) {
  registry.owner = owner;
}

export function getTerminal(id: string) {
  return registry.terminals.get(id);
}

export function listTerminals() {
  return [...registry.terminals.values()];
}

export function statusOf(entry: TerminalEntry): TerminalStatus {
  return { state: entry.state, exitCode: entry.exitCode, lastOutputAt: entry.lastOutputAt };
}

export type CreateOptions = {
  id: string;
  cols: number;
  rows: number;
  /** Defaults to $HOME. */
  cwd?: string;
  /** Defaults to a login shell. */
  command?: string[];
  /** Added to the server's environment. */
  env?: Record<string, string>;
};

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

/** Spawns a terminal. Throws if the process can't be started (missing binary, missing cwd); nothing is registered then. */
export function createTerminal({ id, cols, rows, cwd = process.env.HOME ?? "/", command, env }: CreateOptions) {
  if (registry.terminals.has(id)) throw new Error(`Terminal ${id} already exists`);

  const headless = new HeadlessTerminal({ cols, rows, scrollback: SCROLLBACK, allowProposedApi: true });
  const serializer = new SerializeAddon();
  headless.loadAddon(serializer as never);

  const entry = {
    id,
    headless,
    serializer,
    cols,
    rows,
    cwd,
    state: "running",
    exitCode: null,
    lastOutputAt: null,
    pending: [],
    flushTimer: null,
    exitPublished: false,
    redrawTimer: null,
    reaping: null,
    reaped: false,
  } as unknown as TerminalEntry;

  try {
    entry.proc = Bun.spawn(command ?? [process.env.SHELL ?? "/bin/zsh", "-l"], {
      cwd,
      env: terminalEnv(env),
      terminal: {
        cols,
        rows,
        // Copy: the buffer may be reused after the callback returns, and output is batched.
        data: (_terminal, data) => registry.ops.onData(entry, data.slice()),
      },
    });
  } catch (error) {
    headless.dispose();
    throw error;
  }
  entry.sid = entry.proc.pid;
  entry.proc.exited.then(code => registry.ops.onExit(entry, entry.proc.exitCode ?? code));

  registry.terminals.set(id, entry);
  registry.hooks.changed(id);
  return entry;
}

export function writeInput(id: string, data: string | Uint8Array) {
  const entry = registry.terminals.get(id);
  if (entry?.state === "running") entry.proc.terminal?.write(data);
}

export function resizeTerminal(id: string, cols: number, rows: number) {
  const entry = registry.terminals.get(id);
  if (entry) resize(entry, cols, rows);
}

/**
 * Output written before a resize was produced at the old size, so the headless
 * terminal is resized through its write queue, after everything already in it.
 */
function resize(entry: TerminalEntry, cols: number, rows: number) {
  if (entry.cols === cols && entry.rows === rows) return;
  flush(entry);
  entry.cols = cols;
  entry.rows = rows;
  if (entry.state === "running") entry.proc.terminal?.resize(cols, rows);
  entry.headless.write("", () => entry.headless.resize(cols, rows));
}

/**
 * Resizes the terminal if `size` is given, then serializes the screen and
 * scrollback at that size and calls `deliver` with it. `deliver` runs at an
 * exact point in the output stream: everything published before it is in the
 * snapshot and everything published after it is not, so it must send the
 * snapshot and subscribe the client in the same synchronous step.
 *
 * The snapshot can't capture every bit of terminal state, so afterwards the
 * app is made to redraw.
 */
export function attachTerminal(
  entry: TerminalEntry,
  size: { cols: number; rows: number } | null,
  deliver: (snapshot: string) => void,
) {
  if (size) resize(entry, size.cols, size.rows);
  else flush(entry);
  entry.headless.write("", () => {
    if (!isLive(entry)) return;
    deliver(entry.serializer.serialize());
    forceRedraw(entry);
  });
}

/**
 * Makes the app redraw by taking the PTY one row off its size and back, which
 * sends it SIGWINCH twice. Only the PTY changes: what the app draws in between
 * is parsed at the real size and painted over by the redraw. The restore reads
 * the size when it runs, so a real resize in between wins.
 */
function forceRedraw(entry: TerminalEntry) {
  if (entry.state !== "running" || entry.redrawTimer) return;
  entry.proc.terminal?.resize(entry.cols, entry.rows > 1 ? entry.rows - 1 : entry.rows + 1);
  entry.redrawTimer = setTimeout(() => {
    entry.redrawTimer = null;
    if (entry.state === "running") entry.proc.terminal?.resize(entry.cols, entry.rows);
  }, REDRAW_GAP_MS);
}

function isLive(entry: TerminalEntry) {
  return registry.terminals.get(entry.id) === entry;
}

function scheduleFlush(entry: TerminalEntry) {
  entry.flushTimer ??= setTimeout(() => flush(entry), FLUSH_INTERVAL_MS);
}

/** Passes pending output to the headless terminal, which publishes it once parsed. */
function flush(entry: TerminalEntry) {
  if (entry.flushTimer) clearTimeout(entry.flushTimer);
  entry.flushTimer = null;
  if (entry.pending.length === 0) return;

  const chunk = concat(entry.pending);
  entry.pending = [];
  entry.headless.write(chunk, () => {
    // A closed terminal's id may already belong to a new one.
    if (isLive(entry)) registry.hooks.output(entry, chunk);
  });
}

function concat(chunks: Uint8Array[]) {
  if (chunks.length === 1) return chunks[0]!;
  const out = new Uint8Array(chunks.reduce((total, chunk) => total + chunk.length, 0));
  let offset = 0;
  for (const chunk of chunks) {
    out.set(chunk, offset);
    offset += chunk.length;
  }
  return out;
}

registry.ops = {
  onData(entry, data) {
    if (!isLive(entry)) return;
    entry.pending.push(data);
    entry.lastOutputAt = Date.now();
    registry.hooks.changed(entry.id);
    scheduleFlush(entry);
  },

  onExit(entry, code) {
    if (entry.state === "exited") return;
    entry.state = "exited";
    entry.exitCode = code;
    // Whatever the shell left behind goes now, while its session still has members and so can't be reused.
    const reaping = (entry.reaping ??= reapSession(entry));
    if (!isLive(entry)) return;
    flush(entry);
    // The exit goes out only after the last output has. Once it has, and nothing is left running, the terminal is forgotten.
    entry.headless.write("", () => {
      if (!isLive(entry)) return;
      entry.exitPublished = true;
      registry.hooks.exit(entry);
      void reaping.then(() => forget(entry));
    });
    registry.hooks.changed(entry.id);
  },

  async shutdown() {
    registry.owner.beforeShutdown();
    const closes = listTerminals().map(entry => closeTerminal(entry.id));
    await Promise.all([...closes, ...[...registry.closing].map(entry => entry.reaping)]);
  },

  killAllNow() {
    for (const entry of [...registry.terminals.values(), ...registry.closing]) {
      for (const pid of sessionMembersNow(entry)) signalPid(pid, "SIGKILL");
    }
  },
};

/** Drops an exited terminal whose processes are all gone. Attaching to it afterwards gets `gone`. */
function forget(entry: TerminalEntry) {
  if (!isLive(entry)) return;
  registry.terminals.delete(entry.id);
  registry.hooks.removed(entry.id);
  registry.hooks.changed(entry.id);
  entry.headless.dispose();
  registry.owner.ended(entry);
}

/** Kills a terminal's processes and forgets it. Resolves once everything is gone. */
export async function closeTerminal(id: string) {
  const entry = registry.terminals.get(id);
  if (!entry) return false;
  registry.terminals.delete(id);
  registry.closing.add(entry);
  registry.hooks.removed(id);
  registry.hooks.changed(id);

  for (const timer of [entry.flushTimer, entry.redrawTimer]) if (timer) clearTimeout(timer);
  entry.flushTimer = entry.redrawTimer = null;
  entry.pending = [];

  // Closing the PTY makes the kernel hang up on the foreground job.
  entry.proc.terminal?.close();
  entry.reaping ??= reapSession(entry);
  await entry.reaping;
  await entry.proc.exited;
  entry.headless.dispose();
  registry.closing.delete(entry);
  return true;
}

/**
 * Stops every process in the PTY's session. An interactive shell ignores
 * SIGTERM and runs each job in its own process group, and `nohup`ed jobs
 * outlive it, so this signals the whole session: SIGTERM and SIGHUP, then
 * SIGKILL for whatever is left after a grace period. Processes that left the
 * session with setsid are no longer the terminal's and are not touched.
 */
async function reapSession(entry: TerminalEntry) {
  /** Signals the members and returns how many there were; null if the scan failed, in which case nothing is signalled. */
  const signalMembers = async (...signals: NodeJS.Signals[]) => {
    const pids = await scanMembers(entry);
    for (const pid of pids ?? []) for (const signal of signals) signalPid(pid, signal);
    return pids?.length ?? null;
  };

  if (!(await signalMembers("SIGTERM", "SIGHUP"))) return;
  const deadline = Date.now() + KILL_GRACE_MS;
  while (Date.now() < deadline && (await scanMembers(entry))?.length !== 0) await Bun.sleep(KILL_POLL_MS);
  for (let attempts = 0; attempts < 50; attempts++) {
    const left = await signalMembers("SIGKILL");
    if (left === 0) return;
    await Bun.sleep(20);
  }
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

/**
 * Live processes in the terminal's session, wherever they were reparented to,
 * out of `pids`. The kernel does not reuse a session id while the session has
 * members, so a member found while the session has never been seen empty
 * really is ours. Once it is empty, the id is never looked up again. A live
 * process holding the shell's pid after the shell exited also means the id
 * has been reused.
 */
function membersOf(entry: TerminalEntry, pids: number[]) {
  if (entry.reaped) return [];
  const shellExited = entry.proc.exitCode !== null || entry.proc.signalCode !== null;
  const members =
    shellExited && pids.includes(entry.sid)
      ? []
      : pids.filter(pid => pid !== process.pid && pid > 1 && libc.symbols.getsid(pid) === entry.sid);
  if (shellExited && members.length === 0) entry.reaped = true;
  return members;
}

/** The session's members, or null if the process list couldn't be read. */
async function scanMembers(entry: TerminalEntry) {
  if (entry.reaped) return [];
  const pids = await allPids();
  return pids && membersOf(entry, pids);
}

/** Synchronous scan, only for the last-resort kill while the server process exits. */
function sessionMembersNow(entry: TerminalEntry) {
  if (entry.reaped) return [];
  return membersOf(entry, parsePids(Bun.spawnSync(PS).stdout.toString()));
}

function signalPid(pid: number, signal: NodeJS.Signals) {
  try {
    process.kill(pid, signal);
  } catch {
    // Already gone.
  }
}

/** Kills every terminal. Used on SIGINT/SIGTERM. */
export function shutdownTerminals() {
  return registry.ops.shutdown();
}

if (!registry.processHandlersInstalled) {
  registry.processHandlersInstalled = true;

  const onSignal = () => {
    registry.ops.shutdown().finally(() => process.exit(0));
  };
  process.on("SIGINT", onSignal);
  process.on("SIGTERM", onSignal);
  // Last resort if the server dies without a clean shutdown, including terminals mid-close.
  process.on("exit", () => registry.ops.killAllNow());
}
